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
		var in, out, cc, cr int
		if err := rows.Scan(&model, &in, &out, &cc, &cr); err != nil {
			return nil, err
		}
		p.InputTokens += in
		p.OutputTokens += out
		p.CacheCreate += cc
		p.CacheRead += cr
		pricing := e.cfg.PricingFor(model)
		p.CostUSD += CostUSD(pricing, in, out, cc, cr)
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

type ToolDetail struct {
	Name           string `json:"name"`
	TotalCalls     int    `json:"total_calls"`
	SessionsUsedIn int    `json:"sessions_used_in"`
	FirstUsed      string `json:"first_used,omitempty"`
	LastUsed       string `json:"last_used,omitempty"`
	PerProject     []KV   `json:"per_project"`
	PerDayLast30   []KV   `json:"per_day_last_30"`
}

type KV struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func (e *Engine) ToolDetail(ctx context.Context, name string) (*ToolDetail, error) {
	d := &ToolDetail{Name: name}
	row := e.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT session_id),
		       COALESCE(MIN(ts),''), COALESCE(MAX(ts),'')
		FROM tool_calls WHERE name=?`, name)
	if err := row.Scan(&d.TotalCalls, &d.SessionsUsedIn, &d.FirstUsed, &d.LastUsed); err != nil {
		return nil, err
	}
	if d.TotalCalls == 0 {
		return d, nil
	}
	pp, err := e.db.QueryContext(ctx, `
		SELECT s.project_slug, COUNT(*) c
		FROM tool_calls tc JOIN sessions s ON s.id=tc.session_id
		WHERE tc.name=?
		GROUP BY s.project_slug ORDER BY c DESC`, name)
	if err != nil {
		return nil, err
	}
	defer pp.Close()
	for pp.Next() {
		var k KV
		if err := pp.Scan(&k.Key, &k.Count); err != nil {
			return nil, err
		}
		d.PerProject = append(d.PerProject, k)
	}

	pd, err := e.db.QueryContext(ctx, `
		SELECT substr(ts,1,10) d, COUNT(*) c
		FROM tool_calls
		WHERE name=? AND ts >= datetime('now','-30 days')
		GROUP BY d ORDER BY d DESC`, name)
	if err != nil {
		return nil, err
	}
	defer pd.Close()
	for pd.Next() {
		var k KV
		if err := pd.Scan(&k.Key, &k.Count); err != nil {
			return nil, err
		}
		d.PerDayLast30 = append(d.PerDayLast30, k)
	}
	return d, nil
}
