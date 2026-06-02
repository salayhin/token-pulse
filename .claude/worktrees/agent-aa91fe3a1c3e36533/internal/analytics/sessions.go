package analytics

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type SessionSummary struct {
	ID           string  `json:"id"`
	ProjectSlug  string  `json:"project_slug"`
	CWD          string  `json:"cwd,omitempty"`
	GitBranch    string  `json:"git_branch,omitempty"`
	StartedAt    string  `json:"started_at,omitempty"`
	EndedAt      string  `json:"ended_at,omitempty"`
	MessageCount int     `json:"message_count"`
	ToolCalls    int     `json:"tool_calls"`
	CostUSD      float64 `json:"cost_usd"`
	FirstPrompt  string  `json:"first_prompt,omitempty"`
	// Identity fields. DisplayTitle is server-resolved precedence:
	// custom_title > agent_name > ai_title > "" (empty → caller falls back to ID).
	AITitle      string `json:"ai_title,omitempty"`
	CustomTitle  string `json:"custom_title,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
	DisplayTitle string `json:"display_title,omitempty"`
}

func resolveDisplayTitle(custom, agent, ai string) string {
	switch {
	case custom != "":
		return custom
	case agent != "":
		return agent
	case ai != "":
		return ai
	}
	return ""
}

type SessionListResponse struct {
	Sessions   []SessionSummary `json:"sessions"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// Sessions returns a cursor-paginated list, newest first.
// Cursor is opaque base64 of "ended_at|id".
// from/to are inclusive-start, exclusive-end against ended_at; pass zero values to skip.
func (e *Engine) Sessions(ctx context.Context, project, cursor string, from, to time.Time, limit int) (*SessionListResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{}
	conds := []string{"1=1"}
	if project != "" {
		conds = append(conds, "s.project_slug=?")
		args = append(args, project)
	}
	if !from.IsZero() {
		conds = append(conds, "s.ended_at >= ?")
		args = append(args, fmtTS(from))
	}
	if !to.IsZero() {
		conds = append(conds, "s.ended_at < ?")
		args = append(args, fmtTS(to))
	}
	if cursor != "" {
		ts, id, err := decodeCursor(cursor)
		if err == nil {
			conds = append(conds, "(s.ended_at < ? OR (s.ended_at = ? AND s.id < ?))")
			args = append(args, ts, ts, id)
		}
	}
	q := fmt.Sprintf(`
		SELECT s.id, s.project_slug, COALESCE(s.cwd,''), COALESCE(s.git_branch,''),
		       COALESCE(s.started_at,''), COALESCE(s.ended_at,''), s.message_count,
		       (SELECT COUNT(*) FROM tool_calls tc WHERE tc.session_id=s.id) AS tool_calls,
		       COALESCE(s.ai_title,''), COALESCE(s.custom_title,''), COALESCE(s.agent_name,'')
		FROM sessions s
		WHERE %s
		ORDER BY s.ended_at DESC, s.id DESC
		LIMIT ?`, strings.Join(conds, " AND "))
	args = append(args, limit+1)

	rows, err := e.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionSummary
	for rows.Next() {
		var s SessionSummary
		if err := rows.Scan(&s.ID, &s.ProjectSlug, &s.CWD, &s.GitBranch,
			&s.StartedAt, &s.EndedAt, &s.MessageCount, &s.ToolCalls,
			&s.AITitle, &s.CustomTitle, &s.AgentName); err != nil {
			return nil, err
		}
		s.DisplayTitle = resolveDisplayTitle(s.CustomTitle, s.AgentName, s.AITitle)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	resp := &SessionListResponse{Sessions: out}
	if len(out) > limit {
		last := out[limit-1]
		resp.Sessions = out[:limit]
		resp.NextCursor = encodeCursor(last.EndedAt, last.ID)
	}

	for i := range resp.Sessions {
		if cost, err := e.sessionCost(ctx, resp.Sessions[i].ID); err == nil {
			resp.Sessions[i].CostUSD = cost
		}
		resp.Sessions[i].FirstPrompt = e.firstPrompt(ctx, resp.Sessions[i].ID)
	}
	return resp, nil
}

func (e *Engine) sessionCost(ctx context.Context, sessionID string) (float64, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT model,
		       COALESCE(SUM(input_tokens),0),
		       COALESCE(SUM(output_tokens),0),
		       COALESCE(SUM(cache_create_5m_tokens),0),
		       COALESCE(SUM(cache_create_1h_tokens),0),
		       COALESCE(SUM(cache_create_tokens),0),
		       COALESCE(SUM(cache_read_tokens),0)
		FROM messages
		WHERE session_id=? AND role='assistant'
		GROUP BY model`, sessionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total float64
	for rows.Next() {
		var model string
		var in, out, c5m, c1h, cLegacy, cr int
		if err := rows.Scan(&model, &in, &out, &c5m, &c1h, &cLegacy, &cr); err != nil {
			return 0, err
		}
		total += CostUSD(e.cfg.PricingFor(model), in, out, c5m, c1h, cLegacy, cr)
	}
	return total, rows.Err()
}

func (e *Engine) firstPrompt(ctx context.Context, sessionID string) string {
	var preview string
	_ = e.db.QueryRowContext(ctx, `
		SELECT COALESCE(preview,'') FROM messages
		WHERE session_id=? AND role='user' AND text != ''
		ORDER BY ts ASC LIMIT 1`, sessionID).Scan(&preview)
	return preview
}

type SessionMessage struct {
	UUID         string            `json:"uuid"`
	ParentUUID   *string           `json:"parent_uuid,omitempty"`
	Role         string            `json:"role"`
	Model        string            `json:"model,omitempty"`
	Ts           string            `json:"ts"`
	Text         string            `json:"text,omitempty"`
	Preview      string            `json:"preview,omitempty"`
	HasThinking  bool              `json:"has_thinking,omitempty"`
	InputTokens  int               `json:"input_tokens,omitempty"`
	OutputTokens int               `json:"output_tokens,omitempty"`
	CacheCreate  int               `json:"cache_create_tokens,omitempty"`
	CacheRead    int               `json:"cache_read_tokens,omitempty"`
	CostUSD      float64           `json:"cost_usd,omitempty"`
	ToolCalls    []ToolCallSummary `json:"tool_calls,omitempty"`
}

type ToolCallSummary struct {
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id,omitempty"`
}

type SessionDetail struct {
	Session  SessionSummary   `json:"session"`
	Cache    *CacheStats      `json:"cache,omitempty"`
	Messages []SessionMessage `json:"messages"`
}

func (e *Engine) Session(ctx context.Context, sessionID string) (*SessionDetail, error) {
	var s SessionSummary
	row := e.db.QueryRowContext(ctx, `
		SELECT id, project_slug, COALESCE(cwd,''), COALESCE(git_branch,''),
		       COALESCE(started_at,''), COALESCE(ended_at,''), message_count,
		       COALESCE(ai_title,''), COALESCE(custom_title,''), COALESCE(agent_name,'')
		FROM sessions WHERE id=?`, sessionID)
	if err := row.Scan(&s.ID, &s.ProjectSlug, &s.CWD, &s.GitBranch,
		&s.StartedAt, &s.EndedAt, &s.MessageCount,
		&s.AITitle, &s.CustomTitle, &s.AgentName); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	s.DisplayTitle = resolveDisplayTitle(s.CustomTitle, s.AgentName, s.AITitle)

	cost, err := e.sessionCost(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	s.CostUSD = cost

	rows, err := e.db.QueryContext(ctx, `
		SELECT uuid, parent_uuid, role, COALESCE(model,''), ts,
		       COALESCE(text,''), COALESCE(preview,''), has_thinking,
		       input_tokens, output_tokens,
		       cache_create_tokens, cache_create_5m_tokens, cache_create_1h_tokens,
		       cache_read_tokens
		FROM messages WHERE session_id=?
		ORDER BY ts ASC, uuid ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []SessionMessage
	for rows.Next() {
		var m SessionMessage
		var hasThinking, c5m, c1h int
		if err := rows.Scan(&m.UUID, &m.ParentUUID, &m.Role, &m.Model, &m.Ts,
			&m.Text, &m.Preview, &hasThinking,
			&m.InputTokens, &m.OutputTokens,
			&m.CacheCreate, &c5m, &c1h,
			&m.CacheRead); err != nil {
			return nil, err
		}
		m.HasThinking = hasThinking != 0
		if m.Role == "assistant" {
			p := e.cfg.PricingFor(m.Model)
			m.CostUSD = CostUSD(p, m.InputTokens, m.OutputTokens, c5m, c1h, m.CacheCreate, m.CacheRead)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach tool calls per message.
	tcRows, err := e.db.QueryContext(ctx, `
		SELECT message_uuid, name, COALESCE(tool_use_id,'') FROM tool_calls WHERE session_id=?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer tcRows.Close()
	tcByMsg := map[string][]ToolCallSummary{}
	for tcRows.Next() {
		var mu, name, tid string
		if err := tcRows.Scan(&mu, &name, &tid); err != nil {
			return nil, err
		}
		tcByMsg[mu] = append(tcByMsg[mu], ToolCallSummary{Name: name, ToolUseID: tid})
	}
	for i := range msgs {
		msgs[i].ToolCalls = tcByMsg[msgs[i].UUID]
	}
	s.ToolCalls = countToolCalls(tcByMsg)
	s.FirstPrompt = e.firstPrompt(ctx, sessionID)
	cache, err := e.sessionCache(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return &SessionDetail{Session: s, Cache: cache, Messages: msgs}, nil
}

func (e *Engine) sessionCache(ctx context.Context, sessionID string) (*CacheStats, error) {
	row := e.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(input_tokens),0),
		        COALESCE(SUM(cache_create_tokens),0),
		        COALESCE(SUM(cache_read_tokens),0)
		 FROM messages WHERE session_id=? AND role='assistant'`, sessionID)
	var in, cc, cr int
	if err := row.Scan(&in, &cc, &cr); err != nil {
		return nil, err
	}
	hr := 0.0
	if denom := float64(in + cc + cr); denom > 0 {
		hr = float64(cr) / denom
	}

	rows, err := e.db.QueryContext(ctx, `
		SELECT model,
		       COALESCE(SUM(cache_create_5m_tokens),0),
		       COALESCE(SUM(cache_create_1h_tokens),0),
		       COALESCE(SUM(cache_create_tokens),0),
		       COALESCE(SUM(cache_read_tokens),0)
		FROM messages WHERE session_id=? AND role='assistant'
		GROUP BY model`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var savings float64
	for rows.Next() {
		var model string
		var c5m, c1h, cLegacy, r int
		if err := rows.Scan(&model, &c5m, &c1h, &cLegacy, &r); err != nil {
			return nil, err
		}
		savings += NetCacheSavingsUSD(e.cfg.PricingFor(model), c5m, c1h, cLegacy, r)
	}
	return &CacheStats{
		InputTokens:       in,
		CacheCreateTokens: cc,
		CacheReadTokens:   cr,
		HitRate:           hr,
		NetSavingsUSD:     savings,
	}, rows.Err()
}

func countToolCalls(m map[string][]ToolCallSummary) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}

// Cursor helpers --------------------------------------------------------

func encodeCursor(ts, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(ts + "|" + id))
}

func decodeCursor(cursor string) (string, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("bad cursor")
	}
	return parts[0], parts[1], nil
}
