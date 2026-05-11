package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateFreshDB verifies that opening a brand-new DB applies every
// migration exactly once and stamps schema_version accordingly.
func TestMigrateFreshDB(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "fresh.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	versions := loadVersions(t, s.DB())
	if len(versions) != len(migrations) {
		t.Fatalf("expected %d migration rows, got %d (%v)", len(migrations), len(versions), versions)
	}
	for i, m := range migrations {
		if versions[i] != m.version {
			t.Fatalf("version[%d] = %d, want %d", i, versions[i], m.version)
		}
	}
}

// TestMigrateIdempotent verifies that reopening an already-migrated DB does
// not re-apply any migration (schema_version row count stays constant).
func TestMigrateIdempotent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "idem.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	before := loadVersions(t, s1.DB())
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("open #2: %v", err)
	}
	defer s2.Close()
	after := loadVersions(t, s2.DB())

	if len(before) != len(after) {
		t.Fatalf("re-open changed schema_version rows: %v -> %v", before, after)
	}
}

// TestMigrateV2HealsDuplicates simulates a legacy DB that has v1 applied
// but not v2, with duplicate tool_calls rows. Opening it should run v2,
// dedup the rows, and create the unique index.
func TestMigrateV2HealsDuplicates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Build a "legacy" state: open once, then rip out v2's effects so we
	// can verify the runner re-applies the heal step.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db := s.DB()
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_tool_calls_unique`); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM schema_version WHERE version >= 2`); err != nil {
		t.Fatalf("rewind schema_version: %v", err)
	}
	// Seed a session row that tool_calls.session_id can reference.
	if _, err := db.Exec(`INSERT INTO sessions(id, project_slug, started_at) VALUES('s1', 'p', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// Insert two duplicates of (message_uuid, tool_use_id).
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(`INSERT INTO tool_calls(message_uuid, session_id, tool_use_id, name, ts)
			VALUES('m1','s1','toolu_dup','Bash',CURRENT_TIMESTAMP)`); err != nil {
			t.Fatalf("seed duplicate: %v", err)
		}
	}
	s.Close()

	// Reopen — v2 should run, dedup, and create the index.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.DB().QueryRow(`SELECT COUNT(*) FROM tool_calls WHERE tool_use_id='toolu_dup'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 deduped row, got %d", count)
	}
	versions := loadVersions(t, s2.DB())
	hasV2 := false
	for _, v := range versions {
		if v == 2 {
			hasV2 = true
			break
		}
	}
	if !hasV2 {
		t.Fatalf("v2 not recorded after reopen: %v", versions)
	}
}

func loadVersions(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_version ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	return out
}
