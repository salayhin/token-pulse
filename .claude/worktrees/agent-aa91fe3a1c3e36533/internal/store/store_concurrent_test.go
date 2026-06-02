package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestConcurrentWrites simulates the watcher-vs-startup-index contention
// scenario that produces "context deadline exceeded" in production: many
// writers fighting for the SQLite single-writer slot while readers run in
// parallel. With the writeMu + chunked InsertMessages in place, all writers
// should complete cleanly and reads should never see a stale connection.
func TestConcurrentWrites(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	// Seed sessions that the messages reference (foreign keys are on).
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.UpsertSession(ctx, SessionRow{
			ID:           fmt.Sprintf("s%d", i),
			ProjectSlug:  "p",
			StartedAt:    time.Now(),
			EndedAt:      time.Now(),
			MessageCount: 200,
		})
		cancel()
		if err != nil {
			t.Fatalf("seed session %d: %v", i, err)
		}
	}

	const writers, perWriter = 8, 200
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	start := time.Now()

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			rows := make([]MessageRow, perWriter)
			now := time.Now()
			for i := range rows {
				rows[i] = MessageRow{
					UUID:        fmt.Sprintf("w%d-r%d", w, i),
					SessionID:   fmt.Sprintf("s%d", w),
					ProjectSlug: "p",
					Role:        "user",
					Ts:          now,
					Text:        strings.Repeat("x", 2048),
				}
			}
			if err := s.InsertMessages(ctx, rows); err != nil {
				errCh <- fmt.Errorf("writer %d: %w", w, err)
			}
		}(w)
	}

	// Concurrent reader: hammers SELECT while writes happen. Should never
	// fail; with WAL mode reads are non-blocking, but we want to be sure no
	// driver-level error surfaces.
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			var n int
			err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages`).Scan(&n)
			cancel()
			if err != nil {
				errCh <- fmt.Errorf("reader: %w", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	if d := time.Since(start); d > 25*time.Second {
		t.Errorf("took too long: %s", d)
	}

	// Sanity: rows landed.
	var got int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if want := writers * perWriter; got != want {
		t.Errorf("got %d messages, want %d", got, want)
	}
}

// TestChunkedInsertMessages verifies that InsertMessages correctly splits a
// batch larger than insertMessagesBatch into multiple commits. Regression
// test for B3.
func TestChunkedInsertMessages(t *testing.T) {
	t.Parallel()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.UpsertSession(ctx, SessionRow{
		ID: "sess", ProjectSlug: "p",
		StartedAt: time.Now(), EndedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 1.5x the chunk size to force a multi-chunk path.
	const total = insertMessagesBatch + insertMessagesBatch/2
	rows := make([]MessageRow, total)
	now := time.Now()
	for i := range rows {
		rows[i] = MessageRow{
			UUID:        fmt.Sprintf("m-%d", i),
			SessionID:   "sess",
			ProjectSlug: "p",
			Role:        "user",
			Ts:          now,
			Text:        "hello",
		}
	}
	if err := s.InsertMessages(ctx, rows); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id='sess'`).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != total {
		t.Errorf("got %d, want %d", got, total)
	}
}
