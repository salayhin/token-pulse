package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/sirajus-salayhin/tokenpulse/internal/parser"
	"github.com/sirajus-salayhin/tokenpulse/internal/store"
)

type Indexer struct {
	store     *store.Store
	claudeDir string
	log       *slog.Logger
}

func New(s *store.Store, claudeDir string, log *slog.Logger) *Indexer {
	return &Indexer{store: s, claudeDir: claudeDir, log: log}
}

type Stats struct {
	FilesScanned   int
	FilesSkipped   int
	FilesIndexed   int
	MessagesAdded  int
	ToolCallsAdded int
	Duration       time.Duration
}

// Run walks claudeDir/projects and ingests every JSONL session.
// It is incremental at two levels: files unchanged since last run (size+mtime)
// are skipped entirely; files that grew are parsed only from their last
// resume offset, so each watcher reindex of an active session does O(new
// messages) work instead of rebuilding the session from scratch.
func (idx *Indexer) Run(ctx context.Context, force bool) (*Stats, error) {
	start := time.Now()
	st := &Stats{}
	projectsDir := filepath.Join(idx.claudeDir, "projects")

	files, err := parser.ListSessionFiles(projectsDir)
	if err != nil {
		return st, fmt.Errorf("list session files: %w", err)
	}
	st.FilesScanned = len(files)

	for _, path := range files {
		if ctx.Err() != nil {
			return st, ctx.Err()
		}
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		mtime := fi.ModTime().UTC().Truncate(time.Second)

		state, err := idx.store.FileState(ctx, path)
		if err != nil {
			idx.log.Warn("file state lookup failed", "path", path, "err", err)
			continue
		}
		if !force && state.Found && state.Size == fi.Size() && state.Mtime.Equal(mtime) {
			st.FilesSkipped++
			continue
		}

		n, tc, err := idx.indexFile(ctx, path, fi.Size(), mtime, state, force)
		if err != nil {
			idx.log.Warn("index file failed", "path", path, "err", err)
			continue
		}
		st.FilesIndexed++
		st.MessagesAdded += n
		st.ToolCallsAdded += tc
	}
	st.Duration = time.Since(start)
	return st, nil
}

// indexFile ingests a single session file. Two modes:
//
//   - Incremental: state.Found && size >= state.Size && !force. Parses only
//     bytes past state.LastOffset and skips the DeleteSession + full
//     UpsertSession write storm. Cost is proportional to *new* records, not
//     total session size.
//   - Full rebuild: first-time file, size shrunk (truncation/rotation), or
//     force=true. Drops any prior rows for the session and reparses from 0.
func (idx *Indexer) indexFile(
	ctx context.Context,
	path string,
	size int64,
	mtime time.Time,
	state store.FileState,
	force bool,
) (int, int, error) {
	slug := parser.ProjectSlugFromPath(path)

	incremental := !force && state.Found && size >= state.Size && state.LastOffset > 0
	fromOffset := int64(0)
	if incremental {
		fromOffset = state.LastOffset
	}

	var (
		msgs            []store.MessageRow
		tools           []store.ToolCallRow
		parsedSessionID string
		cwd             string
		gitBranch       string
		version         string
		startedAt       time.Time
		endedAt         time.Time
		aiTitle         string
		customTitle     string
		agentName       string
	)

	newOffset, err := parser.ParseFile(path, fromOffset, func(rec *parser.Record) error {
		if rec.SessionID != "" && parsedSessionID == "" {
			parsedSessionID = rec.SessionID
		}
		if rec.CWD != "" {
			cwd = rec.CWD
		}
		if rec.GitBranch != "" {
			gitBranch = rec.GitBranch
		}
		if rec.Version != "" {
			version = rec.Version
		}
		if !rec.Timestamp.IsZero() {
			if startedAt.IsZero() || rec.Timestamp.Before(startedAt) {
				startedAt = rec.Timestamp
			}
			if rec.Timestamp.After(endedAt) {
				endedAt = rec.Timestamp
			}
		}

		switch rec.Type {
		case parser.TypeAssistant:
			am, err := parser.DecodeAssistant(rec)
			if err != nil {
				return nil
			}
			text := parser.AssistantText(am)
			row := store.MessageRow{
				UUID:                rec.UUID,
				SessionID:           rec.SessionID,
				ProjectSlug:         slug,
				ParentUUID:          rec.ParentUUID,
				Role:                "assistant",
				Model:               am.Model,
				Ts:                  rec.Timestamp,
				InputTokens:         am.Usage.InputTokens,
				OutputTokens:        am.Usage.OutputTokens,
				CacheCreateTokens:   am.Usage.CacheCreationInputTokens,
				CacheCreate5mTokens: am.Usage.CacheCreation.Ephemeral5mInputTokens,
				CacheCreate1hTokens: am.Usage.CacheCreation.Ephemeral1hInputTokens,
				CacheReadTokens:     am.Usage.CacheReadInputTokens,
				ServiceTier:         am.Usage.ServiceTier,
				Text:                text,
				Preview:             parser.Preview(text),
			}
			for _, c := range am.Content {
				if c.Type == "thinking" {
					row.HasThinking = true
				}
				if c.Type == "tool_use" && c.Name != "" {
					tools = append(tools, store.ToolCallRow{
						MessageUUID: rec.UUID,
						SessionID:   rec.SessionID,
						ToolUseID:   c.ID,
						Name:        c.Name,
						Ts:          rec.Timestamp,
					})
				}
			}
			msgs = append(msgs, row)
		case parser.TypeUser:
			um, err := parser.DecodeUser(rec)
			if err != nil {
				return nil
			}
			role := "user"
			text := parser.UserText(um)
			if len(um.Content) > 0 && um.Content[0] == '[' && text == "" {
				role = "user-tool-result"
			}
			msgs = append(msgs, store.MessageRow{
				UUID:        rec.UUID,
				SessionID:   rec.SessionID,
				ProjectSlug: slug,
				ParentUUID:  rec.ParentUUID,
				Role:        role,
				Ts:          rec.Timestamp,
				Text:        text,
				Preview:     parser.Preview(text),
			})
		case parser.TypeAITitle:
			if rec.AITitle != "" {
				aiTitle = rec.AITitle
			}
		case parser.TypeCustomTitle:
			if rec.CustomTitle != "" {
				customTitle = rec.CustomTitle
			}
		case parser.TypeAgentName:
			if rec.AgentName != "" {
				agentName = rec.AgentName
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	// Resolve the session this batch of records belongs to. Prefer what we
	// just parsed; fall back to the previously persisted ID for incremental
	// runs where the appended chunk happened to not include sessionId on
	// any record (rare — Claude tags most records — but cheap to defend).
	sessionID := parsedSessionID
	if sessionID == "" {
		sessionID = state.SessionID
	}
	if sessionID == "" {
		// No session info anywhere. Persist offset so we don't rescan the
		// junk and bail.
		_ = idx.store.MarkFileSeen(ctx, path, size, mtime, newOffset, "")
		return 0, 0, nil
	}

	if !incremental {
		// Full rebuild: drop stale rows ONLY when the session ID hasn't
		// changed. If state.SessionID differs from the new sessionID, the
		// file now represents a different logical session — the old one's
		// history stays in the DB rather than getting silently nuked.
		if state.Found && state.SessionID != "" && state.SessionID == sessionID {
			if err := idx.store.DeleteSession(ctx, sessionID); err != nil {
				return 0, 0, err
			}
		}
		if err := idx.store.UpsertProject(ctx, slug, cwd); err != nil {
			return 0, 0, err
		}
		if err := idx.store.UpsertSession(ctx, store.SessionRow{
			ID:           sessionID,
			ProjectSlug:  slug,
			CWD:          cwd,
			GitBranch:    gitBranch,
			Version:      version,
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			MessageCount: len(msgs),
			AITitle:      aiTitle,
			CustomTitle:  customTitle,
			AgentName:    agentName,
		}); err != nil {
			return 0, 0, err
		}
	}

	if err := idx.store.InsertMessages(ctx, msgs); err != nil {
		return 0, 0, err
	}
	if err := idx.store.InsertToolCalls(ctx, tools); err != nil {
		return 0, 0, err
	}

	if incremental {
		// On incremental: refresh ended_at + message_count from the actual
		// table state. Project/session base metadata is left as-is.
		if err := idx.store.BumpSessionActivity(ctx, sessionID, endedAt); err != nil {
			return 0, 0, err
		}
		// Title records normally appear once near session start; on rare
		// occasion a custom-title is set mid-session and lands in an
		// incremental batch — write it through without disturbing other fields.
		if err := idx.store.UpdateSessionTitles(ctx, sessionID, aiTitle, customTitle, agentName); err != nil {
			return 0, 0, err
		}
	}

	if err := idx.store.MarkFileSeen(ctx, path, size, mtime, newOffset, sessionID); err != nil {
		return 0, 0, err
	}
	return len(msgs), len(tools), nil
}
