package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sirajus-salayhin/tokenpulse/internal/analytics"
	"github.com/sirajus-salayhin/tokenpulse/internal/config"
	"github.com/sirajus-salayhin/tokenpulse/internal/indexer"
	"github.com/sirajus-salayhin/tokenpulse/internal/server/handlers"
	"github.com/sirajus-salayhin/tokenpulse/web"
)

type Server struct {
	cfg      *config.Config
	provider *config.Provider
	log      *slog.Logger
	eng      *analytics.Engine
	bus      *handlers.EventBus
	idx      *indexer.Indexer
	srv      *http.Server
}

func New(provider *config.Provider, configPath string, eng *analytics.Engine, bus *handlers.EventBus, health handlers.HealthInfo, log *slog.Logger, idx *indexer.Indexer) *Server {
	cfg := provider.Get()
	s := &Server{cfg: cfg, provider: provider, log: log, eng: eng, bus: bus, idx: idx}
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(requestLog(log))
	r.Use(middleware.Recoverer)

	h := handlers.New(eng, bus, health, idx)
	sh := handlers.NewSettingsHandler(provider, configPath, eng)
	r.Route("/api/v1", func(r chi.Router) {
		// Bounded routes: a 15s per-request timeout cancels r.Context() if
		// the SQLite layer is contended, so handlers fail fast with an
		// observable error instead of hanging until the client gives up.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(15 * time.Second))
			r.Get("/stats", h.Stats)
			r.Get("/stats/daily", h.Daily)
			r.Get("/stats/trends", h.Trends)
			r.Get("/stats/projections", h.Projection)
			r.Get("/cache", h.Cache)
			r.Get("/budget", h.Budget)
			r.Get("/skills", h.Skills)
			r.Get("/projects", h.Projects)
			r.Get("/projects/{slug}/stats", h.ProjectStats)
			r.Get("/sessions", h.Sessions)
			r.Get("/sessions/{id}", h.SessionDetail)
			r.Get("/sessions/{sessionId}/skills", h.SessionSkills)
			r.Get("/prompts", h.PromptStats)
			r.Get("/models", h.Models)
			r.Get("/health", h.Health)
			r.Get("/settings", sh.Get)
			r.Put("/settings", sh.Put)
			r.Get("/rebuild", h.RebuildStatus)
			r.Post("/rebuild", h.Rebuild)
		})
		// Streaming routes must outlive the per-request timeout: SSE keeps
		// connections open indefinitely; export streams large CSV/JSON.
		r.Get("/export", h.Export)
		r.Get("/events", h.Events)
	})

	// Embedded SPA
	subFS, _ := fs.Sub(web.Assets(), ".")
	r.Handle("/static/*", http.StripPrefix("/", noCacheHeaders(http.FileServer(http.FS(subFS)))))
	r.Get("/", spaIndex(subFS))

	s.srv = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return s
}

func (s *Server) Start(ctx context.Context) error {
	s.log.Info("server listening", "addr", s.srv.Addr)
	errCh := make(chan error, 1)
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		// Tell SSE handlers to exit before Shutdown waits on them.
		s.bus.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func spaIndex(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := fsys.Open("index.html")
		if err != nil {
			http.Error(w, "index missing", http.StatusInternalServerError)
			return
		}
		defer func() { _ = f.Close() }()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = copyAll(w, f)
	}
}

func copyAll(dst http.ResponseWriter, src fs.File) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

func noCacheHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func requestLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Debug("http", "method", r.Method, "path", r.URL.Path,
				"status", ww.Status(), "dur_ms", time.Since(start).Milliseconds())
		})
	}
}
