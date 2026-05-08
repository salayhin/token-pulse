package analytics

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

type ExportScope string

const (
	ScopeDaily    ExportScope = "daily"
	ScopeSessions ExportScope = "sessions"
	ScopeTools    ExportScope = "tools"
	ScopeProjects ExportScope = "projects"
)

type ExportFormat string

const (
	FormatCSV  ExportFormat = "csv"
	FormatJSON ExportFormat = "json"
)

func (e *Engine) Export(ctx context.Context, scope ExportScope, format ExportFormat, w io.Writer) error {
	switch scope {
	case ScopeDaily:
		rows, err := e.Daily(ctx, 365)
		if err != nil {
			return err
		}
		return writeOut(format, w, rows, dailyToCSV)
	case ScopeSessions:
		all, err := e.allSessions(ctx)
		if err != nil {
			return err
		}
		return writeOut(format, w, all, sessionsToCSV)
	case ScopeTools:
		rows, err := e.Tools(ctx, 200)
		if err != nil {
			return err
		}
		return writeOut(format, w, rows, toolsToCSV)
	case ScopeProjects:
		rows, err := e.Projects(ctx)
		if err != nil {
			return err
		}
		return writeOut(format, w, rows, projectsToCSV)
	}
	return fmt.Errorf("unknown scope: %s", scope)
}

func (e *Engine) allSessions(ctx context.Context) ([]SessionSummary, error) {
	var out []SessionSummary
	cursor := ""
	for {
		resp, err := e.Sessions(ctx, "", cursor, time.Time{}, time.Time{}, 200)
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Sessions...)
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	return out, nil
}

func writeOut[T any](format ExportFormat, w io.Writer, rows []T, csvFn func(*csv.Writer, []T) error) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case FormatCSV:
		cw := csv.NewWriter(w)
		defer cw.Flush()
		return csvFn(cw, rows)
	}
	return fmt.Errorf("unknown format: %s", format)
}

func dailyToCSV(w *csv.Writer, rows []DailyRow) error {
	if err := w.Write([]string{
		"date", "sessions", "messages", "input_tokens", "output_tokens",
		"cache_create_tokens", "cache_create_5m_tokens", "cache_create_1h_tokens",
		"cache_read_tokens", "cost_usd", "net_cache_savings_usd",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.Date, itoa(r.Sessions), itoa(r.Messages),
			itoa(r.InputTokens), itoa(r.OutputTokens),
			itoa(r.CacheCreateTokens), itoa(r.CacheCreate5mTokens), itoa(r.CacheCreate1hTokens),
			itoa(r.CacheReadTokens),
			ftoa(r.CostUSD), ftoa(r.NetCacheSavingsUSD),
		}); err != nil {
			return err
		}
	}
	return nil
}

func sessionsToCSV(w *csv.Writer, rows []SessionSummary) error {
	if err := w.Write([]string{
		"id", "title", "custom_title", "agent_name", "ai_title",
		"project_slug", "git_branch", "started_at", "ended_at",
		"message_count", "tool_calls", "cost_usd", "first_prompt",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.ID, r.DisplayTitle, r.CustomTitle, r.AgentName, r.AITitle,
			r.ProjectSlug, r.GitBranch, r.StartedAt, r.EndedAt,
			itoa(r.MessageCount), itoa(r.ToolCalls), ftoa(r.CostUSD), r.FirstPrompt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func toolsToCSV(w *csv.Writer, rows []ToolStat) error {
	if err := w.Write([]string{"name", "count"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{r.Name, itoa(r.Count)}); err != nil {
			return err
		}
	}
	return nil
}

func projectsToCSV(w *csv.Writer, rows []ProjectStat) error {
	if err := w.Write([]string{"slug", "cwd", "sessions", "messages", "tool_calls", "input_tokens", "output_tokens", "cache_create_tokens", "cache_read_tokens", "cost_usd", "last_active"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			r.Slug, r.CWD, itoa(r.Sessions), itoa(r.Messages), itoa(r.ToolCalls),
			itoa(r.InputTokens), itoa(r.OutputTokens),
			itoa(r.CacheCreate), itoa(r.CacheRead),
			ftoa(r.CostUSD), r.LastActive,
		}); err != nil {
			return err
		}
	}
	return nil
}

func itoa(n int) string     { return strconv.Itoa(n) }
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }
