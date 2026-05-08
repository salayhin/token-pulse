package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/sirajus-salayhin/claude-token-lens/internal/indexer"
)

type Notifier interface {
	Publish(eventType string, data any)
}

type Watcher struct {
	idx      *indexer.Indexer
	bus      Notifier
	root     string
	log      *slog.Logger
	debounce time.Duration

	mu      sync.Mutex
	pending bool
}

func New(idx *indexer.Indexer, bus Notifier, claudeDir string, log *slog.Logger) *Watcher {
	return &Watcher{
		idx:      idx,
		bus:      bus,
		root:     filepath.Join(claudeDir, "projects"),
		log:      log,
		debounce: 800 * time.Millisecond,
	}
}

// Run watches the projects directory recursively. New project subdirectories
// are picked up as they appear. On any .jsonl change/create, a debounced
// re-index runs and an SSE event is published.
func (w *Watcher) Run(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	if err := w.addExisting(fw); err != nil {
		w.log.Warn("watcher: initial walk failed", "err", err)
	}

	w.log.Info("watcher started", "root", w.root)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			w.handle(fw, ev)
		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.log.Warn("watcher error", "err", err)
		}
	}
}

func (w *Watcher) addExisting(fw *fsnotify.Watcher) error {
	if _, err := os.Stat(w.root); err != nil {
		return err
	}
	if err := fw.Add(w.root); err != nil {
		return err
	}
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			_ = fw.Add(filepath.Join(w.root, e.Name()))
		}
	}
	return nil
}

func (w *Watcher) handle(fw *fsnotify.Watcher, ev fsnotify.Event) {
	// New project directory: start watching it.
	if ev.Op&fsnotify.Create != 0 {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			_ = fw.Add(ev.Name)
			return
		}
	}
	if !strings.HasSuffix(ev.Name, ".jsonl") {
		return
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return
	}
	w.scheduleReindex()
}

func (w *Watcher) scheduleReindex() {
	w.mu.Lock()
	if w.pending {
		w.mu.Unlock()
		w.log.Debug("watcher: reindex skipped (in flight)")
		return
	}
	w.pending = true
	w.mu.Unlock()

	go func() {
		// Clear the in-flight flag only AFTER Run() completes. Otherwise a
		// burst of file events during a long re-index would each spawn their
		// own goroutine and stack writers on the SQLite single-writer slot,
		// each waiting up to busy_timeout repeatedly until the 5-min ctx
		// fires as "context deadline exceeded".
		defer func() {
			w.mu.Lock()
			w.pending = false
			w.mu.Unlock()
		}()

		time.Sleep(w.debounce)

		start := time.Now()
		w.log.Debug("watcher: reindex start")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		stats, err := w.idx.Run(ctx, false)
		if err != nil {
			w.log.Warn("watcher: reindex failed", "err", err, "dur", time.Since(start))
			return
		}
		w.log.Debug("watcher: reindex end",
			"dur", time.Since(start),
			"files_indexed", stats.FilesIndexed,
			"messages", stats.MessagesAdded)
		if stats.FilesIndexed > 0 || stats.MessagesAdded > 0 {
			w.log.Info("watcher reindexed",
				"files", stats.FilesIndexed,
				"messages", stats.MessagesAdded,
				"dur", stats.Duration)
			w.bus.Publish("updated", map[string]any{
				"files_indexed":  stats.FilesIndexed,
				"messages_added": stats.MessagesAdded,
				"ts":             time.Now().UTC(),
			})
		}
	}()
}

// InFlight reports whether a debounced reindex is scheduled or running.
// Surfaced on the /health endpoint for runtime diagnostics.
func (w *Watcher) InFlight() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pending
}
