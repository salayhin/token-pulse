package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sirajus-salayhin/tokenpulse/internal/analytics"
)

// HealthInfo bundles optional runtime introspection sources surfaced on the
// /health endpoint. All fields are nil-safe; missing sources are simply
// omitted from the response.
type HealthInfo struct {
	DB         *sql.DB
	InFlight   func() bool
	SlowWrites func() int64
}

type Handlers struct {
	eng    *analytics.Engine
	bus    *EventBus
	health HealthInfo
}

func New(eng *analytics.Engine, bus *EventBus, health HealthInfo) *Handlers {
	return &Handlers{eng: eng, bus: bus, health: health}
}

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	resp, err := h.eng.Stats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

func (h *Handlers) Daily(w http.ResponseWriter, r *http.Request) {
	days := intParam(r, "days", 30)
	rows, err := h.eng.Daily(r.Context(), days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"daily": rows})
}

func (h *Handlers) Trends(w http.ResponseWriter, r *http.Request) {
	days := intParam(r, "days", 30)
	pts, err := h.eng.Trends(r.Context(), days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"trends": pts})
}

func (h *Handlers) Projection(w http.ResponseWriter, r *http.Request) {
	p, err := h.eng.Projection(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, p)
}

func (h *Handlers) Cache(w http.ResponseWriter, r *http.Request) {
	c, err := h.eng.Cache(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, c)
}


func (h *Handlers) Projects(w http.ResponseWriter, r *http.Request) {
	ps, err := h.eng.Projects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"projects": ps})
}

func (h *Handlers) ProjectStats(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, err := h.eng.ProjectStats(r.Context(), slug)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, p)
}

func (h *Handlers) Sessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	project := q.Get("project")
	cursor := q.Get("cursor")
	limit := intParam(r, "limit", 50)
	// from/to accept "YYYY-MM-DD" (UTC). The range is half-open: from <= ended_at < to+1d,
	// so an end-date selection includes the entire day.
	var from, to time.Time
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = t.Add(24 * time.Hour)
		}
	}
	resp, err := h.eng.Sessions(r.Context(), project, cursor, from, to, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, resp)
}

func (h *Handlers) SessionDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := h.eng.Session(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, d)
}

func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := analytics.ExportFormat(q.Get("format"))
	if format == "" {
		format = analytics.FormatJSON
	}
	scope := analytics.ExportScope(q.Get("scope"))
	if scope == "" {
		scope = analytics.ScopeDaily
	}
	switch format {
	case analytics.FormatCSV:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="export-`+string(scope)+`.csv"`)
	case analytics.FormatJSON:
		w.Header().Set("Content-Type", "application/json")
	default:
		writeErr(w, http.StatusBadRequest, "format must be csv or json")
		return
	}
	if err := h.eng.Export(r.Context(), scope, format, w); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handlers) PromptStats(w http.ResponseWriter, r *http.Request) {
	s, err := h.eng.PromptStats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, s)
}

func (h *Handlers) Models(w http.ResponseWriter, r *http.Request) {
	m, err := h.eng.Models(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"models": m})
}

func (h *Handlers) Skills(w http.ResponseWriter, r *http.Request) {
	result, err := h.eng.SkillsBreakdown(r.Context(), "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result)
}

func (h *Handlers) SessionSkills(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	result, err := h.eng.SkillsBreakdown(r.Context(), sessionID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, result)
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"status": "ok",
		"ts":     time.Now().UTC(),
	}
	if h.health.DB != nil {
		s := h.health.DB.Stats()
		out["db"] = map[string]any{
			"in_use":        s.InUse,
			"idle":          s.Idle,
			"open":          s.OpenConnections,
			"max_open":      s.MaxOpenConnections,
			"wait_count":    s.WaitCount,
			"wait_duration": s.WaitDuration.String(),
		}
	}
	if h.health.InFlight != nil {
		out["watcher_in_flight"] = h.health.InFlight()
	}
	if h.health.SlowWrites != nil {
		out["slow_writes"] = h.health.SlowWrites()
	}
	writeJSON(w, out)
}

func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := h.bus.Subscribe()
	defer h.bus.Unsubscribe(ch)

	// Initial hello.
	writeSSE(w, "ready", map[string]any{"ts": time.Now().UTC()})
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.bus.Done():
			return
		case ev := <-ch:
			writeSSE(w, ev.Type, ev.Data)
			flusher.Flush()
		case <-ticker.C:
			// keep-alive comment
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

// Helpers ---------------------------------------------------------------

func intParam(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeSSE(w http.ResponseWriter, event string, data any) {
	b, _ := json.Marshal(data)
	_, _ = w.Write([]byte("event: " + event + "\ndata: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
}
