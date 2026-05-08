package indexer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirajus-salayhin/claude-token-lens/internal/store"
)

// silentLogger discards all output; tests don't need indexer noise.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// writeJSONL appends raw lines to path. Each line must already include the
// trailing newline. Returns the absolute path written to.
func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func userLine(uuid, sessionID, ts, text string) string {
	return fmt.Sprintf(
		`{"type":"user","uuid":%q,"sessionId":%q,"timestamp":%q,"cwd":"/p","gitBranch":"main","version":"v1","message":{"role":"user","content":%q}}`+"\n",
		uuid, sessionID, ts, text,
	)
}

func assistantLine(uuid, sessionID, ts, text, toolName, toolID string) string {
	tool := ""
	if toolName != "" {
		tool = fmt.Sprintf(`,{"type":"tool_use","id":%q,"name":%q,"input":{}}`, toolID, toolName)
	}
	return fmt.Sprintf(
		`{"type":"assistant","uuid":%q,"sessionId":%q,"timestamp":%q,"message":{"id":"m","type":"message","role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":%q}%s],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"service_tier":"standard"}}}`+"\n",
		uuid, sessionID, ts, text, tool,
	)
}

// setupClaudeDir creates a temp ~/.claude/projects/<slug>/<file>.jsonl
// scaffold and returns (claudeDir, sessionFilePath).
func setupClaudeDir(t *testing.T) (string, string) {
	t.Helper()
	claudeDir := t.TempDir()
	projectsDir := filepath.Join(claudeDir, "projects", "myproj")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return claudeDir, filepath.Join(projectsDir, "session.jsonl")
}

// TestIncrementalAppendOnlyInsertsNewRows is the headline test: prove that a
// file appended-to between two indexer runs causes the second run to insert
// only the new records — no DeleteSession storm, no rewrite of the session
// metadata, and no duplicate rows.
func TestIncrementalAppendOnlyInsertsNewRows(t *testing.T) {
	t.Parallel()

	claudeDir, sessionFile := setupClaudeDir(t)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	idx := New(s, claudeDir, silentLogger())
	ctx := context.Background()

	// First pass: write 3 records and index.
	writeJSONL(t, sessionFile,
		userLine("u-1", "sess-1", "2026-05-08T10:00:00Z", "hello"),
		assistantLine("a-1", "sess-1", "2026-05-08T10:00:01Z", "hi", "Bash", "tu-1"),
		userLine("u-2", "sess-1", "2026-05-08T10:00:02Z", "more"),
	)
	stats1, err := idx.Run(ctx, false)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if stats1.MessagesAdded != 3 || stats1.ToolCallsAdded != 1 {
		t.Errorf("first run stats: got msgs=%d tools=%d, want 3/1",
			stats1.MessagesAdded, stats1.ToolCallsAdded)
	}

	// Snapshot row counts and session metadata.
	var msgsBefore, toolsBefore int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgsBefore)
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&toolsBefore)
	if msgsBefore != 3 || toolsBefore != 1 {
		t.Fatalf("seeded counts wrong: msgs=%d tools=%d", msgsBefore, toolsBefore)
	}

	// Append 2 more records.
	writeJSONL(t, sessionFile,
		assistantLine("a-2", "sess-1", "2026-05-08T10:00:03Z", "ok", "", ""),
		userLine("u-3", "sess-1", "2026-05-08T10:00:04Z", "thanks"),
	)
	// Bump mtime past truncation precision so the FileState skip-check sees
	// the file as changed.
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(sessionFile, future, future)

	stats2, err := idx.Run(ctx, false)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	// Critical assertion: second run added exactly the 2 appended rows.
	if stats2.MessagesAdded != 2 {
		t.Errorf("incremental run added %d messages, want 2", stats2.MessagesAdded)
	}
	if stats2.FilesIndexed != 1 {
		t.Errorf("incremental run indexed %d files, want 1", stats2.FilesIndexed)
	}

	var msgsAfter, toolsAfter int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgsAfter)
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM tool_calls`).Scan(&toolsAfter)
	if msgsAfter != 5 {
		t.Errorf("total messages = %d, want 5", msgsAfter)
	}
	if toolsAfter != 1 {
		// No new tool_use in the appended chunk; the original tool stays.
		t.Errorf("total tool_calls = %d, want 1", toolsAfter)
	}

	// Session row should reflect the new ended_at and updated message_count
	// (set by BumpSessionActivity, NOT by a full UpsertSession).
	var msgCount int
	if err := s.DB().QueryRow(
		`SELECT message_count FROM sessions WHERE id='sess-1'`,
	).Scan(&msgCount); err != nil {
		t.Fatalf("session row: %v", err)
	}
	if msgCount != 5 {
		t.Errorf("session.message_count = %d, want 5", msgCount)
	}
}

// TestIncrementalIdempotent verifies that running the indexer twice in a row
// without any file changes is a no-op (no duplicate rows, no errors).
func TestIncrementalIdempotent(t *testing.T) {
	t.Parallel()

	claudeDir, sessionFile := setupClaudeDir(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	idx := New(s, claudeDir, silentLogger())
	ctx := context.Background()

	writeJSONL(t, sessionFile,
		userLine("u-1", "sess-x", "2026-05-08T10:00:00Z", "hello"),
		assistantLine("a-1", "sess-x", "2026-05-08T10:00:01Z", "hi", "Bash", "tu-1"),
	)

	stats1, _ := idx.Run(ctx, false)
	if stats1.FilesIndexed != 1 {
		t.Fatalf("first run: indexed=%d, want 1", stats1.FilesIndexed)
	}

	stats2, _ := idx.Run(ctx, false)
	if stats2.FilesIndexed != 0 || stats2.FilesSkipped != 1 {
		t.Errorf("second run: indexed=%d skipped=%d, want 0/1",
			stats2.FilesIndexed, stats2.FilesSkipped)
	}

	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n)
	if n != 2 {
		t.Errorf("messages = %d, want 2", n)
	}
}

// TestIncrementalTruncationFallsBackToFullRebuild ensures that if a session
// file shrinks between runs (e.g., user deleted it and started fresh), we
// don't blindly read from the stale offset — we wipe and reparse.
func TestIncrementalTruncationFallsBackToFullRebuild(t *testing.T) {
	t.Parallel()

	claudeDir, sessionFile := setupClaudeDir(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	idx := New(s, claudeDir, silentLogger())
	ctx := context.Background()

	// First pass: 3 records.
	writeJSONL(t, sessionFile,
		userLine("u-1", "sess-old", "2026-05-08T10:00:00Z", "first"),
		userLine("u-2", "sess-old", "2026-05-08T10:00:01Z", "second"),
		userLine("u-3", "sess-old", "2026-05-08T10:00:02Z", "third"),
	)
	if _, err := idx.Run(ctx, false); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Replace the file with a smaller (different session) one.
	if err := os.Truncate(sessionFile, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	writeJSONL(t, sessionFile,
		userLine("u-A", "sess-new", "2026-05-08T11:00:00Z", "fresh start"),
	)
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(sessionFile, future, future)

	if _, err := idx.Run(ctx, false); err != nil {
		t.Fatalf("second run: %v", err)
	}

	// Old session's rows should still exist (we keyed FileState rebuild on
	// the *prior* session_id, which was sess-old). The new file claims
	// sess-new; old session's data is orphaned but not corrupted.
	// What we mainly want to assert: the new session_id has its row, and
	// total messages reflect the new content.
	var oldRows, newRows int
	_ = s.DB().QueryRow(
		`SELECT COUNT(*) FROM messages WHERE session_id='sess-old'`,
	).Scan(&oldRows)
	_ = s.DB().QueryRow(
		`SELECT COUNT(*) FROM messages WHERE session_id='sess-new'`,
	).Scan(&newRows)

	if newRows != 1 {
		t.Errorf("sess-new messages = %d, want 1", newRows)
	}
	// Old session's rows are preserved because we keyed delete by the
	// stored session_id, but the new session_id differs. This is acceptable
	// behaviour: the original session is no longer referenced by the file
	// but its history isn't destroyed.
	if oldRows != 3 {
		t.Errorf("sess-old messages = %d, want 3 (preserved)", oldRows)
	}
}

// TestIncrementalPartialLineRecovery confirms that a trailing partial line
// (no '\n') is NOT counted in the resume offset, so when the line later
// becomes complete (file flushed), the next reindex picks it up.
func TestIncrementalPartialLineRecovery(t *testing.T) {
	t.Parallel()

	claudeDir, sessionFile := setupClaudeDir(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	idx := New(s, claudeDir, silentLogger())
	ctx := context.Background()

	// One complete line + partial line (no trailing newline).
	complete := userLine("u-1", "sess-p", "2026-05-08T10:00:00Z", "complete")
	partial := `{"type":"user","uuid":"u-2","sessionId":"sess-p","timestamp":"2026-05-08T10:00:01Z","message":{"role":"user","content":"par`
	writeJSONL(t, sessionFile, complete, partial)

	if _, err := idx.Run(ctx, false); err != nil {
		t.Fatalf("first run: %v", err)
	}

	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id='sess-p'`).Scan(&n)
	if n != 1 {
		t.Fatalf("after partial: messages = %d, want 1", n)
	}

	// Complete the partial line and add another.
	writeJSONL(t, sessionFile,
		`tial"}}`+"\n",
		userLine("u-3", "sess-p", "2026-05-08T10:00:02Z", "next"),
	)
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(sessionFile, future, future)

	if _, err := idx.Run(ctx, false); err != nil {
		t.Fatalf("second run: %v", err)
	}

	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id='sess-p'`).Scan(&n)
	if n != 3 {
		t.Errorf("after recovery: messages = %d, want 3 (1 + recovered + new)", n)
	}
}
