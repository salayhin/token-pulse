package analytics

import "context"

type PromptStatsResponse struct {
	UserPrompts        int     `json:"user_prompts"`
	AvgPromptChars     int     `json:"avg_prompt_chars"`
	MedianPromptChars  int     `json:"median_prompt_chars"`
	AssistantReplies   int     `json:"assistant_replies"`
	AvgReplyChars      int     `json:"avg_reply_chars"`
	MedianReplyChars   int     `json:"median_reply_chars"`
	PromptToReplyRatio float64 `json:"prompt_to_reply_ratio"`
}

// PromptStats returns prompt/reply length stats over all messages with text.
func (e *Engine) PromptStats(ctx context.Context) (*PromptStatsResponse, error) {
	userLens, err := e.lengths(ctx, "user")
	if err != nil {
		return nil, err
	}
	asstLens, err := e.lengths(ctx, "assistant")
	if err != nil {
		return nil, err
	}
	r := &PromptStatsResponse{
		UserPrompts:       len(userLens),
		AvgPromptChars:    avgInt(userLens),
		MedianPromptChars: medianInt(userLens),
		AssistantReplies:  len(asstLens),
		AvgReplyChars:     avgInt(asstLens),
		MedianReplyChars:  medianInt(asstLens),
	}
	if r.AvgReplyChars > 0 {
		r.PromptToReplyRatio = float64(r.AvgPromptChars) / float64(r.AvgReplyChars)
	}
	return r, nil
}

func (e *Engine) lengths(ctx context.Context, role string) ([]int, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT length(text) FROM messages WHERE role=? AND text IS NOT NULL AND text != '' ORDER BY length(text)`,
		role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func avgInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	var sum int
	for _, x := range xs {
		sum += x
	}
	return sum / len(xs)
}

func medianInt(xs []int) int {
	n := len(xs)
	if n == 0 {
		return 0
	}
	// xs is already ORDER BY length(text)
	if n%2 == 1 {
		return xs[n/2]
	}
	return (xs[n/2-1] + xs[n/2]) / 2
}

// ModelStat is per-model usage + cost.
type ModelStat struct {
	Model             string  `json:"model"`
	Sessions          int     `json:"sessions"`
	AssistantMessages int     `json:"assistant_messages"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CacheCreate       int     `json:"cache_create_tokens"`
	CacheRead         int     `json:"cache_read_tokens"`
	CostUSD           float64 `json:"cost_usd"`
	AvgCostPerMsgUSD  float64 `json:"avg_cost_per_msg_usd"`
}

func (e *Engine) Models(ctx context.Context) ([]ModelStat, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT COALESCE(model,'<unknown>'),
		       COUNT(DISTINCT session_id),
		       COUNT(*),
		       COALESCE(SUM(input_tokens),0),
		       COALESCE(SUM(output_tokens),0),
		       COALESCE(SUM(cache_create_tokens),0),
		       COALESCE(SUM(cache_read_tokens),0)
		FROM messages
		WHERE role='assistant'
		GROUP BY model
		ORDER BY COUNT(*) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelStat
	for rows.Next() {
		var m ModelStat
		if err := rows.Scan(&m.Model, &m.Sessions, &m.AssistantMessages,
			&m.InputTokens, &m.OutputTokens, &m.CacheCreate, &m.CacheRead); err != nil {
			return nil, err
		}
		p := e.cfg.PricingFor(m.Model)
		m.CostUSD = CostUSD(p, m.InputTokens, m.OutputTokens, m.CacheCreate, m.CacheRead)
		if m.AssistantMessages > 0 {
			m.AvgCostPerMsgUSD = m.CostUSD / float64(m.AssistantMessages)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
