package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// insertMessagesBatch caps the number of rows in a single InsertMessages
// transaction. Keeping each tx short releases the SQLite writer slot
// frequently, so concurrent writers (e.g. the watcher) and readers don't
// stack up behind a multi-second batch.
const insertMessagesBatch = 500

// slowWriteThreshold is the duration above which a store write is logged at
// warn level and counted in the slow-write metric.
const slowWriteThreshold = time.Second

type Store struct {
	db  *sql.DB
	log *slog.Logger // optional; nil-safe via the timed() helper.

	// writeMu serialises every write at the application layer. SQLite already
	// enforces a single writer at the engine level, but without this mutex
	// concurrent writers would each occupy a Go pool connection while waiting
	// on busy_timeout, exhausting the pool and surfacing as "context deadline
	// exceeded" once the caller's ctx fires. Holding writeMu *outside* of
	// BeginTx keeps idle connections free for readers.
	writeMu sync.Mutex

	slowWrites atomic.Int64
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir storage: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(30000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// WAL mode supports many concurrent readers + one writer. Allowing more than
	// one connection is critical: otherwise a long-running indexer write will
	// starve incoming HTTP read queries and they'll abort with
	// "context deadline exceeded" once the browser cancels.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }

// WithLogger attaches a slog logger used for write-timing diagnostics.
// Safe to call once at startup; not goroutine-safe with concurrent writes.
func (s *Store) WithLogger(log *slog.Logger) { s.log = log }

// SlowWrites returns the count of writes that exceeded slowWriteThreshold.
// Useful for surfacing on /health.
func (s *Store) SlowWrites() int64 { return s.slowWrites.Load() }

// timed runs fn while measuring duration. Logs at debug always; promotes to
// warn and bumps the slow-write counter when the call exceeds the threshold.
func (s *Store) timed(op string, fn func() error) error {
	start := time.Now()
	err := fn()
	d := time.Since(start)
	if d >= slowWriteThreshold {
		s.slowWrites.Add(1)
		if s.log != nil {
			s.log.Warn("store slow write", "op", op, "dur", d, "err", err)
		}
	} else if s.log != nil {
		s.log.Debug("store write", "op", op, "dur", d)
	}
	return err
}

// migration is one versioned, atomic schema change. Each fn runs inside its
// own transaction; on success the runner records the version in
// schema_version so the migration never runs twice. New schema changes go at
// the bottom of the migrations slice — never edit a migration that has
// shipped, since existing DBs have already applied it and won't replay it.
type migration struct {
	version int
	name    string
	fn      func(context.Context, *sql.Tx) error
}

// migrations is the ordered list of schema versions. Append-only.
var migrations = []migration{
	{1, "initial_schema", migrateV1InitialSchema},
	{2, "dedup_tool_calls_unique", migrateV2DedupToolCallsUnique},
}

func (s *Store) migrate() error {
	ctx := context.Background()
	// Bootstrap schema_version. Existing DBs may have the legacy
	// single-column form (version INTEGER PRIMARY KEY) created by the old
	// migrate() but never populated; tolerate that by ADDing the new
	// columns with duplicate-column-error suppression.
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL DEFAULT '',
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("migrate: bootstrap schema_version: %w", err)
	}
	for _, alter := range []string{
		`ALTER TABLE schema_version ADD COLUMN name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE schema_version ADD COLUMN applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP`,
	} {
		if _, err := s.db.ExecContext(ctx, alter); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate: schema_version alter: %w (%s)", err, alter)
		}
	}
	var current int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("migrate: read current version: %w", err)
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate v%d %s: begin: %w", m.version, m.name, err)
	}
	defer tx.Rollback()
	if err := m.fn(ctx, tx); err != nil {
		return fmt.Errorf("migrate v%d %s: %w", m.version, m.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_version(version, name, applied_at) VALUES(?,?,?)`,
		m.version, m.name, time.Now().UTC()); err != nil {
		return fmt.Errorf("migrate v%d %s: record version: %w", m.version, m.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate v%d %s: commit: %w", m.version, m.name, err)
	}
	if s.log != nil {
		s.log.Info("migration applied", "version", m.version, "name", m.name)
	}
	return nil
}

// migrateV1InitialSchema brings a DB up to the baseline schema that shipped
// before versioned migrations existed. All statements are idempotent so the
// step is a no-op on DBs that already have these tables (i.e. every existing
// install on disk before v2 was introduced).
func migrateV1InitialSchema(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			slug TEXT PRIMARY KEY,
			cwd  TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id           TEXT PRIMARY KEY,
			project_slug TEXT NOT NULL,
			cwd          TEXT,
			git_branch   TEXT,
			version      TEXT,
			started_at   DATETIME,
			ended_at     DATETIME,
			message_count INTEGER DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_slug)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at)`,
		`CREATE TABLE IF NOT EXISTS messages (
			uuid          TEXT PRIMARY KEY,
			session_id    TEXT NOT NULL,
			parent_uuid   TEXT,
			role          TEXT NOT NULL,
			model         TEXT,
			ts            DATETIME NOT NULL,
			input_tokens  INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_create_tokens INTEGER DEFAULT 0,
			cache_read_tokens   INTEGER DEFAULT 0,
			service_tier  TEXT,
			has_thinking  INTEGER DEFAULT 0,
			text          TEXT,
			preview       TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(ts)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			text,
			message_uuid UNINDEXED,
			session_id   UNINDEXED,
			role         UNINDEXED,
			ts           UNINDEXED,
			project_slug UNINDEXED,
			tokenize='porter unicode61'
		)`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			message_uuid TEXT NOT NULL,
			session_id  TEXT NOT NULL,
			tool_use_id TEXT,
			name        TEXT NOT NULL,
			ts          DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_name ON tool_calls(name)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_ts ON tool_calls(ts)`,
		`CREATE TABLE IF NOT EXISTS files_seen (
			path     TEXT PRIMARY KEY,
			size     INTEGER NOT NULL,
			mtime    DATETIME NOT NULL,
			indexed_at DATETIME NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("%w (%s)", err, q)
		}
	}
	// SQLite has no ADD COLUMN IF NOT EXISTS, so swallow "duplicate column"
	// for compatibility with DBs that already have these columns from
	// pre-versioned migrations.
	for _, alter := range []string{
		`ALTER TABLE files_seen ADD COLUMN last_offset INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE files_seen ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`,
		// Cache-create tokens split by ephemeral TTL — see CostUSD in analytics.
		`ALTER TABLE messages ADD COLUMN cache_create_5m_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN cache_create_1h_tokens INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sessions ADD COLUMN ai_title     TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN custom_title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN agent_name   TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := tx.ExecContext(ctx, alter); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("%w (%s)", err, alter)
		}
	}
	return nil
}

// migrateV2DedupToolCallsUnique removes legacy duplicate (message_uuid,
// tool_use_id) rows that accumulated before the unique index existed, then
// adds the partial unique index. INSERT OR IGNORE in InsertToolCalls relies
// on this index for incremental reindex idempotency.
func migrateV2DedupToolCallsUnique(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM tool_calls
		WHERE tool_use_id != ''
		  AND id NOT IN (
		    SELECT MIN(id) FROM tool_calls
		    WHERE tool_use_id != ''
		    GROUP BY message_uuid, tool_use_id
		  )`); err != nil {
		return fmt.Errorf("dedup tool_calls: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_calls_unique
		ON tool_calls(message_uuid, tool_use_id)
		WHERE tool_use_id != ''`); err != nil {
		return fmt.Errorf("create unique index: %w", err)
	}
	return nil
}

// MessageRow is the input shape for batch upserts.
type MessageRow struct {
	UUID        string
	SessionID   string
	ProjectSlug string
	ParentUUID  *string
	Role        string
	Model       string
	Ts          time.Time
	InputTokens int
	// CacheCreateTokens is the wire-format sum (5m + 1h). CacheCreate5mTokens
	// and CacheCreate1hTokens carry the split when present.
	CacheCreateTokens   int
	CacheCreate5mTokens int
	CacheCreate1hTokens int
	CacheReadTokens     int
	OutputTokens        int
	ServiceTier         string
	HasThinking         bool
	Text                string
	Preview             string
}

type ToolCallRow struct {
	MessageUUID string
	SessionID   string
	ToolUseID   string
	Name        string
	Ts          time.Time
}

type SessionRow struct {
	ID           string
	ProjectSlug  string
	CWD          string
	GitBranch    string
	Version      string
	StartedAt    time.Time
	EndedAt      time.Time
	MessageCount int
	// Identity / display fields. Empty string means "not seen in this batch";
	// UpsertSession preserves any prior non-empty value via COALESCE-on-empty.
	AITitle     string
	CustomTitle string
	AgentName   string
}

func (s *Store) UpsertProject(ctx context.Context, slug, cwd string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("UpsertProject", func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO projects(slug, cwd) VALUES(?,?)
			 ON CONFLICT(slug) DO UPDATE SET cwd=excluded.cwd`, slug, cwd)
		return err
	})
}

func (s *Store) UpsertSession(ctx context.Context, r SessionRow) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("UpsertSession", func() error {
		// Title fields: an incremental reindex of an active session may not
		// re-encounter the ai-title / custom-title / agent-name records (they
		// appear once near session start). We must NOT overwrite a stored
		// title with an empty incoming value — hence NULLIF + COALESCE.
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO sessions(id, project_slug, cwd, git_branch, version, started_at, ended_at, message_count, ai_title, custom_title, agent_name)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT(id) DO UPDATE SET
			   cwd=excluded.cwd,
			   git_branch=excluded.git_branch,
			   version=excluded.version,
			   started_at=MIN(sessions.started_at, excluded.started_at),
			   ended_at=MAX(sessions.ended_at, excluded.ended_at),
			   message_count=excluded.message_count,
			   ai_title=COALESCE(NULLIF(excluded.ai_title, ''), sessions.ai_title),
			   custom_title=COALESCE(NULLIF(excluded.custom_title, ''), sessions.custom_title),
			   agent_name=COALESCE(NULLIF(excluded.agent_name, ''), sessions.agent_name)`,
			r.ID, r.ProjectSlug, r.CWD, r.GitBranch, r.Version,
			fmtTS(r.StartedAt), fmtTS(r.EndedAt), r.MessageCount,
			r.AITitle, r.CustomTitle, r.AgentName)
		return err
	})
}

// fmtTS formats a time.Time as a SQLite-friendly ISO 8601 string in UTC.
// SQLite's strftime() parses this format natively.
func fmtTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05.000")
}

// InsertMessages inserts message rows in chunks of insertMessagesBatch. Each
// chunk runs in its own short-lived transaction so the writer slot is held
// for sub-second windows even on multi-thousand-row session files.
func (s *Store) InsertMessages(ctx context.Context, rows []MessageRow) error {
	if len(rows) == 0 {
		return nil
	}
	for i := 0; i < len(rows); i += insertMessagesBatch {
		end := i + insertMessagesBatch
		if end > len(rows) {
			end = len(rows)
		}
		if err := s.insertMessagesChunk(ctx, rows[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) insertMessagesChunk(ctx context.Context, rows []MessageRow) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("InsertMessages.chunk", func() error {
		tBegin := time.Now()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if s.log != nil {
			s.log.Debug("tx begin", "op", "InsertMessages.chunk", "rows", len(rows), "wait", time.Since(tBegin))
		}
		defer tx.Rollback()
		stmt, err := tx.PrepareContext(ctx,
			`INSERT OR IGNORE INTO messages
			 (uuid, session_id, parent_uuid, role, model, ts, input_tokens, output_tokens, cache_create_tokens, cache_create_5m_tokens, cache_create_1h_tokens, cache_read_tokens, service_tier, has_thinking, text, preview)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		ftsStmt, err := tx.PrepareContext(ctx,
			`INSERT INTO messages_fts(text, message_uuid, session_id, role, ts, project_slug) VALUES(?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer ftsStmt.Close()
		for _, r := range rows {
			if _, err := stmt.ExecContext(ctx,
				r.UUID, r.SessionID, r.ParentUUID, r.Role, r.Model, fmtTS(r.Ts),
				r.InputTokens, r.OutputTokens,
				r.CacheCreateTokens, r.CacheCreate5mTokens, r.CacheCreate1hTokens,
				r.CacheReadTokens,
				r.ServiceTier, boolToInt(r.HasThinking), r.Text, r.Preview,
			); err != nil {
				return err
			}
			if r.Text != "" {
				if _, err := ftsStmt.ExecContext(ctx, r.Text, r.UUID, r.SessionID, r.Role, fmtTS(r.Ts), r.ProjectSlug); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

func (s *Store) InsertToolCalls(ctx context.Context, rows []ToolCallRow) error {
	if len(rows) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("InsertToolCalls", func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		// INSERT OR IGNORE leans on idx_tool_calls_unique so a re-read of
		// the boundary record on incremental reindex is a no-op.
		stmt, err := tx.PrepareContext(ctx,
			`INSERT OR IGNORE INTO tool_calls(message_uuid, session_id, tool_use_id, name, ts)
			 VALUES(?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range rows {
			if _, err := stmt.ExecContext(ctx, r.MessageUUID, r.SessionID, r.ToolUseID, r.Name, fmtTS(r.Ts)); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

// FileState captures the indexer's resume point for a session file.
// LastOffset is the byte position after the last complete line that has
// already been ingested. SessionID is the session this file belongs to —
// stashed here so incremental reindex doesn't have to re-parse the head of
// the file just to recover it.
type FileState struct {
	Found      bool
	Size       int64
	Mtime      time.Time
	LastOffset int64
	SessionID  string
}

// FileState returns the previously persisted state for path, or a zero
// FileState{Found:false} if the file has never been indexed.
func (s *Store) FileState(ctx context.Context, path string) (FileState, error) {
	var st FileState
	err := s.db.QueryRowContext(ctx,
		`SELECT size, mtime, last_offset, session_id FROM files_seen WHERE path=?`, path,
	).Scan(&st.Size, &st.Mtime, &st.LastOffset, &st.SessionID)
	if err == sql.ErrNoRows {
		return FileState{}, nil
	}
	if err != nil {
		return FileState{}, err
	}
	st.Found = true
	return st, nil
}

func (s *Store) MarkFileSeen(ctx context.Context, path string, size int64, mtime time.Time, lastOffset int64, sessionID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("MarkFileSeen", func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO files_seen(path, size, mtime, indexed_at, last_offset, session_id)
			 VALUES(?,?,?,?,?,?)
			 ON CONFLICT(path) DO UPDATE SET
			   size=excluded.size,
			   mtime=excluded.mtime,
			   indexed_at=excluded.indexed_at,
			   last_offset=excluded.last_offset,
			   session_id=CASE WHEN excluded.session_id != '' THEN excluded.session_id ELSE files_seen.session_id END`,
			path, size, mtime, time.Now().UTC(), lastOffset, sessionID)
		return err
	})
}

// UpdateSessionTitles writes the title fields for a session, preserving any
// already-stored value when the incoming string is empty. Used by the
// incremental indexing path because ai-title / custom-title / agent-name
// records are typically emitted near session start and won't reappear in
// later batches; a generic UpsertSession on incremental would risk wiping
// cwd/git_branch with empty strings.
func (s *Store) UpdateSessionTitles(ctx context.Context, sessionID, aiTitle, customTitle, agentName string) error {
	if aiTitle == "" && customTitle == "" && agentName == "" {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("UpdateSessionTitles", func() error {
		_, err := s.db.ExecContext(ctx,
			`UPDATE sessions
			   SET ai_title     = COALESCE(NULLIF(?, ''), ai_title),
			       custom_title = COALESCE(NULLIF(?, ''), custom_title),
			       agent_name   = COALESCE(NULLIF(?, ''), agent_name)
			 WHERE id = ?`,
			aiTitle, customTitle, agentName, sessionID)
		return err
	})
}

// BumpSessionActivity refreshes ended_at and message_count without touching
// the session's other metadata. Used by the incremental indexing path: when
// only new records are appended, we don't want to rewrite cwd / git_branch
// / version / started_at — just acknowledge the session is still alive and
// keep the count in sync with the messages table.
func (s *Store) BumpSessionActivity(ctx context.Context, sessionID string, endedAt time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("BumpSessionActivity", func() error {
		_, err := s.db.ExecContext(ctx,
			`UPDATE sessions
			   SET ended_at = MAX(COALESCE(ended_at, ''), ?),
			       message_count = (SELECT COUNT(*) FROM messages WHERE session_id = ?)
			 WHERE id = ?`,
			fmtTS(endedAt), sessionID, sessionID)
		return err
	})
}

// DeleteSession removes a session and its messages/tool_calls (used when re-indexing a changed file).
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.timed("DeleteSession", func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, q := range []string{
			`DELETE FROM tool_calls WHERE session_id=?`,
			`DELETE FROM messages WHERE session_id=?`,
			`DELETE FROM messages_fts WHERE session_id=?`,
			`DELETE FROM sessions WHERE id=?`,
		} {
			if _, err := tx.ExecContext(ctx, q, sessionID); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
