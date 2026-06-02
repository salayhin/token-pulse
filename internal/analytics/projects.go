package analytics

import (
	"context"
	"database/sql"
)

type ProjectStat struct {
	Slug         string  `json:"slug"`
	CWD          string  `json:"cwd"`
	Sessions     int     `json:"sessions"`
	Messages     int     `json:"messages"`
	ToolCalls    int     `json:"tool_calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheCreate  int     `json:"cache_create_tokens"`
	CacheRead    int     `json:"cache_read_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	LastActive   string  `json:"last_active,omitempty"`
}

func (e *Engine) Projects(ctx context.Context) ([]ProjectStat, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT p.slug, COALESCE(p.cwd,''),
		       COUNT(DISTINCT s.id),
		       COALESCE(MAX(s.ended_at), '')
		FROM projects p
		LEFT JOIN sessions s ON s.project_slug = p.slug
		GROUP BY p.slug
		ORDER BY MAX(s.ended_at) DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectStat
	for rows.Next() {
		var p ProjectStat
		if err := rows.Scan(&p.Slug, &p.CWD, &p.Sessions, &p.LastActive); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Per-project cost is a second query so per-model pricing applies.
	for i := range out {
		stat, err := e.fillProjectStat(ctx, &out[i])
		if err != nil {
			return nil, err
		}
		out[i] = *stat
	}
	return out, nil
}

func (e *Engine) ProjectStats(ctx context.Context, slug string) (*ProjectStat, error) {
	row := e.db.QueryRowContext(ctx, `
		SELECT slug, COALESCE(cwd,''), 0, ''
		FROM projects WHERE slug=?`, slug)
	p := &ProjectStat{}
	if err := row.Scan(&p.Slug, &p.CWD, &p.Sessions, &p.LastActive); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return e.fillProjectStat(ctx, p)
}

func (e *Engine) fillProjectStat(ctx context.Context, p *ProjectStat) (*ProjectStat, error) {
	// Sessions count + last active.
	row := e.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(MAX(ended_at),'') FROM sessions WHERE project_slug=?`, p.Slug)
	if err := row.Scan(&p.Sessions, &p.LastActive); err != nil {
		return nil, err
	}

	// Per-model token rollup → cost.
	rows, err := e.db.QueryContext(ctx, `
		SELECT m.model,
		       COALESCE(SUM(m.input_tokens),0),
		       COALESCE(SUM(m.output_tokens),0),
		       COALESCE(SUM(m.cache_create_5m_tokens),0),
		       COALESCE(SUM(m.cache_create_1h_tokens),0),
		       COALESCE(SUM(m.cache_create_tokens),0),
		       COALESCE(SUM(m.cache_read_tokens),0)
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.role='assistant' AND s.project_slug=?
		GROUP BY m.model`, p.Slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var model string
		var in, out, c5m, c1h, cLegacy, cr int
		if err := rows.Scan(&model, &in, &out, &c5m, &c1h, &cLegacy, &cr); err != nil {
			return nil, err
		}
		p.InputTokens += in
		p.OutputTokens += out
		p.CacheCreate += cLegacy
		p.CacheRead += cr
		pricing := e.Cfg().PricingFor(model)
		p.CostUSD += CostUSD(pricing, in, out, c5m, c1h, cLegacy, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Message count + tool calls.
	if err := e.db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM messages m JOIN sessions s ON s.id=m.session_id
		   WHERE s.project_slug=? AND m.role IN ('user','assistant')),
		  (SELECT COUNT(*) FROM tool_calls tc JOIN sessions s ON s.id=tc.session_id
		   WHERE s.project_slug=?)`, p.Slug, p.Slug).Scan(&p.Messages, &p.ToolCalls); err != nil {
		return nil, err
	}
	return p, nil
}
