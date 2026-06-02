package analytics

import (
	"context"
	"database/sql"
	"time"

	"github.com/sirajus-salayhin/tokenpulse/internal/config"
)

// Engine reads the live config through a *config.Provider so settings changes
// (timezone, pricing, subscription, budget) take effect on the next request
// without a restart. Direct e.cfg.X reads were removed because they captured
// a snapshot at construction time and went stale after PUT /api/v1/settings.
type Engine struct {
	db       *sql.DB
	provider *config.Provider
}

// New constructs an Engine bound to a live config provider.
func New(db *sql.DB, provider *config.Provider) *Engine {
	return &Engine{db: db, provider: provider}
}

// NewWithConfig is a convenience for CLI subcommands and tests that hold a
// static *config.Config — it wraps the config in a one-shot provider so the
// same Engine type handles both server (live) and CLI (frozen) use.
func NewWithConfig(db *sql.DB, cfg *config.Config) *Engine {
	return New(db, config.NewProvider(cfg))
}

// Cfg returns the *current* config snapshot. Cheap; safe under concurrent
// reads. Callers must treat the returned *Config as immutable.
func (e *Engine) Cfg() *config.Config { return e.provider.Get() }

type Totals struct {
	Sessions           int     `json:"sessions"`
	Messages           int     `json:"messages"`
	AssistantMsgs      int     `json:"assistant_messages"`
	UserMsgs           int     `json:"user_messages"`
	ToolCalls          int     `json:"tool_calls"`
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	CacheCreateTokens  int     `json:"cache_create_tokens"`
	CacheReadTokens    int     `json:"cache_read_tokens"`
	CostUSD            float64 `json:"cost_usd"`
	NetCacheSavingsUSD float64 `json:"net_cache_savings_usd"`
}

type StatsResponse struct {
	Today     Totals    `json:"today"`
	AllTime   Totals    `json:"all_time"`
	Generated time.Time `json:"generated_at"`
	Timezone  string    `json:"timezone"`
}

func (e *Engine) Stats(ctx context.Context) (*StatsResponse, error) {
	cfg := e.Cfg()
	loc := cfg.Location()
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()

	allTime, err := e.totals(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	today, err := e.totals(ctx, startOfDay, time.Time{})
	if err != nil {
		return nil, err
	}
	return &StatsResponse{
		Today:     today,
		AllTime:   allTime,
		Generated: time.Now().UTC(),
		Timezone:  cfg.Timezone,
	}, nil
}

// totals computes aggregates over [from, to). Zero times mean unbounded.
func (e *Engine) totals(ctx context.Context, from, to time.Time) (Totals, error) {
	t := Totals{}
	where, args := buildTimeRange("ts", from, to)
	roleFilter := "role IN ('user','assistant')"
	if where == "" {
		where = " WHERE " + roleFilter
	} else {
		where += " AND " + roleFilter
	}
	q := `SELECT
		COALESCE(COUNT(DISTINCT session_id), 0),
		COALESCE(COUNT(*), 0),
		COALESCE(SUM(CASE WHEN role='assistant' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN role='user' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cache_create_tokens), 0),
		COALESCE(SUM(cache_read_tokens), 0)
	FROM messages` + where
	row := e.db.QueryRowContext(ctx, q, args...)
	if err := row.Scan(&t.Sessions, &t.Messages, &t.AssistantMsgs, &t.UserMsgs,
		&t.InputTokens, &t.OutputTokens, &t.CacheCreateTokens, &t.CacheReadTokens); err != nil {
		return t, err
	}

	tWhere, tArgs := buildTimeRange("ts", from, to)
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_calls`+tWhere, tArgs...).Scan(&t.ToolCalls); err != nil {
		return t, err
	}

	cost, savings, err := e.costAndSavings(ctx, from, to)
	if err != nil {
		return t, err
	}
	t.CostUSD = cost
	t.NetCacheSavingsUSD = savings
	return t, nil
}

// costAndSavings groups by model so per-model pricing applies.
func (e *Engine) costAndSavings(ctx context.Context, from, to time.Time) (float64, float64, error) {
	cfg := e.Cfg()
	where, args := buildTimeRange("ts", from, to)
	q := `SELECT model,
		COALESCE(SUM(input_tokens),0),
		COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(cache_create_5m_tokens),0),
		COALESCE(SUM(cache_create_1h_tokens),0),
		COALESCE(SUM(cache_create_tokens),0),
		COALESCE(SUM(cache_read_tokens),0)
		FROM messages WHERE role='assistant'`
	if where != "" {
		q += " AND " + where[len(" WHERE "):]
	}
	q += " GROUP BY model"
	rows, err := e.db.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	var totalCost, totalSaved float64
	for rows.Next() {
		var model string
		var in, out, c5m, c1h, cLegacy, cr int
		if err := rows.Scan(&model, &in, &out, &c5m, &c1h, &cLegacy, &cr); err != nil {
			return 0, 0, err
		}
		p := cfg.PricingFor(model)
		totalCost += CostUSD(p, in, out, c5m, c1h, cLegacy, cr)
		totalSaved += NetCacheSavingsUSD(p, c5m, c1h, cLegacy, cr)
	}
	return totalCost, totalSaved, rows.Err()
}

type DailyRow struct {
	Date                string  `json:"date"`
	Sessions            int     `json:"sessions"`
	Messages            int     `json:"messages"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheCreateTokens   int     `json:"cache_create_tokens"`
	CacheCreate5mTokens int     `json:"cache_create_5m_tokens"`
	CacheCreate1hTokens int     `json:"cache_create_1h_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	NetCacheSavingsUSD  float64 `json:"net_cache_savings_usd"`
}

func (e *Engine) Daily(ctx context.Context, days int) ([]DailyRow, error) {
	if days <= 0 {
		days = 30
	}
	cfg := e.Cfg()
	loc := cfg.Location()
	// SQLite's strftime is UTC by default; we shift ts into local tz, then truncate to date.
	// Format the offset for SQLite's modifier syntax.
	tzMod := tzModifier(loc)

	rows, err := e.db.QueryContext(ctx, `
		SELECT strftime('%Y-%m-%d', ts, ?) AS d,
		       COUNT(DISTINCT session_id),
		       COUNT(*),
		       COALESCE(SUM(input_tokens),0),
		       COALESCE(SUM(output_tokens),0),
		       COALESCE(SUM(cache_create_tokens),0),
		       COALESCE(SUM(cache_create_5m_tokens),0),
		       COALESCE(SUM(cache_create_1h_tokens),0),
		       COALESCE(SUM(cache_read_tokens),0)
		FROM messages WHERE role='assistant'
		GROUP BY d ORDER BY d DESC LIMIT ?`, tzMod, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyRow
	for rows.Next() {
		var r DailyRow
		if err := rows.Scan(&r.Date, &r.Sessions, &r.Messages, &r.InputTokens, &r.OutputTokens,
			&r.CacheCreateTokens, &r.CacheCreate5mTokens, &r.CacheCreate1hTokens,
			&r.CacheReadTokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Second pass: compute accurate per-day cost using per-model pricing.
	// GROUP_CONCAT(DISTINCT model) returns models in arbitrary order so using
	// the first model's rates for all tokens can be wildly wrong (e.g. haiku
	// rates applied to an opus-heavy day). Instead we issue one extra query
	// that groups by (day, model) — same pattern as costAndSavings.
	costRows, err := e.db.QueryContext(ctx, `
		SELECT strftime('%Y-%m-%d', ts, ?) AS d, model,
		       COALESCE(SUM(input_tokens),0),
		       COALESCE(SUM(output_tokens),0),
		       COALESCE(SUM(cache_create_5m_tokens),0),
		       COALESCE(SUM(cache_create_1h_tokens),0),
		       COALESCE(SUM(cache_create_tokens),0),
		       COALESCE(SUM(cache_read_tokens),0)
		FROM messages WHERE role='assistant'
		GROUP BY d, model
		ORDER BY d DESC`, tzMod)
	if err != nil {
		return nil, err
	}
	defer costRows.Close()

	type dayCost struct{ cost, savings float64 }
	dayMap := make(map[string]*dayCost, len(out))
	for costRows.Next() {
		var d, model string
		var in, out2, c5m, c1h, cLeg, cr int
		if err := costRows.Scan(&d, &model, &in, &out2, &c5m, &c1h, &cLeg, &cr); err != nil {
			return nil, err
		}
		p := cfg.PricingFor(model)
		dc := dayMap[d]
		if dc == nil {
			dc = &dayCost{}
			dayMap[d] = dc
		}
		dc.cost += CostUSD(p, in, out2, c5m, c1h, cLeg, cr)
		dc.savings += NetCacheSavingsUSD(p, c5m, c1h, cLeg, cr)
	}
	if err := costRows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if dc := dayMap[out[i].Date]; dc != nil {
			out[i].CostUSD = dc.cost
			out[i].NetCacheSavingsUSD = dc.savings
		}
	}
	return out, nil
}

// BudgetPeriod is one row of the budget summary: a label, the cost actually
// spent in the period, the pace allotted to the period (derived from the
// configured monthly budget; 0 = no limit), and the period boundaries in the
// user's configured timezone.
type BudgetPeriod struct {
	Period      string    `json:"period"`        // "day", "week", "month"
	CostUSD     float64   `json:"cost_usd"`      // actual spend in the period
	BudgetUSD   float64   `json:"budget_usd"`    // budget for the period (derived for day/week)
	Remaining   float64   `json:"remaining_usd"` // budget - cost; can be negative
	UsedPercent float64   `json:"used_percent"`  // 0..(>100); only meaningful when budget>0
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"` // exclusive upper bound
}

type BudgetResponse struct {
	Day           BudgetPeriod         `json:"day"`
	Week          BudgetPeriod         `json:"week"`
	Month         BudgetPeriod         `json:"month"`
	MonthlyBudget float64              `json:"monthly_budget_usd"` // configured budget (== Month.BudgetUSD)
	DaysInMonth   int                  `json:"days_in_month"`      // calendar days in current month
	DerivedDaily  float64              `json:"derived_daily_usd"`  // monthly / days_in_month
	DerivedWeekly float64              `json:"derived_weekly_usd"` // derived_daily * 7
	Subscription  SubscriptionSummary  `json:"subscription"`
	Timezone      string               `json:"timezone"`
}

// SubscriptionSummary lets the UI render the "subscription value" card
// without re-computing month-to-date spend on the client.
//
// When IsSubscription is true:
//
//	NetBenefitUSD = APIValueMTDUSD - MonthlyFeeUSD   // positive means "getting your money's worth"
//	Multiplier    = APIValueMTDUSD / MonthlyFeeUSD   // 1.0 = breakeven, 5.0 = 5× value
//
// When the plan is "api" (pay-as-you-go), the UI hides the card and uses
// the field only to keep cost labels as "Cost" rather than "API value".
type SubscriptionSummary struct {
	Plan           string  `json:"plan"`            // "api" | "pro" | "max_5x" | "max_20x" | "team" | "custom"
	PlanLabel      string  `json:"plan_label"`      // human-readable: "API", "Pro", "Max 5×", ...
	IsSubscription bool    `json:"is_subscription"` // true ⇒ flat-fee plan; relabel cost figures
	IsFeeFixed     bool    `json:"is_fee_fixed"`    // true ⇒ fee determined by plan (api/pro/max_*); UI hides the input
	MonthlyFeeUSD  float64 `json:"monthly_fee_usd"`
	APIValueMTDUSD float64 `json:"api_value_mtd_usd"`
	NetBenefitUSD  float64 `json:"net_benefit_usd"` // api_value_mtd - monthly_fee
	Multiplier     float64 `json:"multiplier"`      // api_value_mtd / monthly_fee (0 if fee==0)
}

// Budget returns today's, this-week's, and this-month's spend alongside
// budgets derived from the configured monthly budget. Periods are bucketed in
// cfg.Location(); weeks start Monday (ISO 8601). When the monthly budget is 0
// all periods report budget=0 and UsedPercent=0 — the UI suppresses progress
// bars in that case.
//
// Derivation:
//
//	daily_pace  = monthly / days_in_current_calendar_month
//	weekly_pace = daily_pace * 7
//
// Using calendar days (28/29/30/31) keeps the daily pace honest across short
// and long months: a $300/mo budget gives $10/day in June (30d) and ~$9.68/day
// in July (31d).
func (e *Engine) Budget(ctx context.Context) (*BudgetResponse, error) {
	cfg := e.Cfg()
	loc := cfg.Location()
	now := time.Now().In(loc)

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	// Monday-anchored ISO week.
	dow := int(now.Weekday()+6) % 7
	startOfWeek := time.Date(now.Year(), now.Month(), now.Day()-dow, 0, 0, 0, 0, loc)
	endOfWeek := startOfWeek.AddDate(0, 0, 7)

	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)
	daysInMonth := int(endOfMonth.Sub(startOfMonth).Hours() / 24)

	monthly := cfg.Alerts.MonthlyBudgetUSD
	var derivedDaily, derivedWeekly float64
	if monthly > 0 && daysInMonth > 0 {
		derivedDaily = monthly / float64(daysInMonth)
		derivedWeekly = derivedDaily * 7
	}

	dayCost, _, err := e.costAndSavings(ctx, startOfDay.UTC(), time.Time{})
	if err != nil {
		return nil, err
	}
	weekCost, _, err := e.costAndSavings(ctx, startOfWeek.UTC(), time.Time{})
	if err != nil {
		return nil, err
	}
	monthCost, _, err := e.costAndSavings(ctx, startOfMonth.UTC(), time.Time{})
	if err != nil {
		return nil, err
	}

	mk := func(label string, cost, budget float64, start, end time.Time) BudgetPeriod {
		pct := 0.0
		if budget > 0 {
			pct = (cost / budget) * 100
		}
		return BudgetPeriod{
			Period:      label,
			CostUSD:     cost,
			BudgetUSD:   budget,
			Remaining:   budget - cost,
			UsedPercent: pct,
			Start:       start,
			End:         end,
		}
	}
	_, feeFixed := config.CanonicalFee(cfg.Subscription.Plan)
	sub := SubscriptionSummary{
		Plan:           cfg.Subscription.Plan,
		PlanLabel:      cfg.Subscription.PlanLabel(),
		IsSubscription: cfg.Subscription.IsSubscription(),
		IsFeeFixed:     feeFixed,
		MonthlyFeeUSD:  cfg.Subscription.MonthlyFeeUSD,
		APIValueMTDUSD: monthCost,
	}
	if sub.MonthlyFeeUSD > 0 {
		sub.NetBenefitUSD = monthCost - sub.MonthlyFeeUSD
		sub.Multiplier = monthCost / sub.MonthlyFeeUSD
	}

	return &BudgetResponse{
		Day:           mk("day", dayCost, derivedDaily, startOfDay, endOfDay),
		Week:          mk("week", weekCost, derivedWeekly, startOfWeek, endOfWeek),
		Month:         mk("month", monthCost, monthly, startOfMonth, endOfMonth),
		MonthlyBudget: monthly,
		DaysInMonth:   daysInMonth,
		DerivedDaily:  derivedDaily,
		DerivedWeekly: derivedWeekly,
		Subscription:  sub,
		Timezone:      cfg.Timezone,
	}, nil
}

type CacheStats struct {
	CacheCreateTokens int     `json:"cache_create_tokens"`
	CacheReadTokens   int     `json:"cache_read_tokens"`
	InputTokens       int     `json:"input_tokens"`
	HitRate           float64 `json:"hit_rate"`
	NetSavingsUSD     float64 `json:"net_savings_usd"`
}

func (e *Engine) Cache(ctx context.Context) (*CacheStats, error) {
	row := e.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(input_tokens),0),
		        COALESCE(SUM(cache_create_tokens),0),
		        COALESCE(SUM(cache_read_tokens),0)
		 FROM messages WHERE role='assistant'`)
	var in, cc, cr int
	if err := row.Scan(&in, &cc, &cr); err != nil {
		return nil, err
	}
	denom := float64(in + cc + cr)
	hitRate := 0.0
	if denom > 0 {
		hitRate = float64(cr) / denom
	}
	_, savings, err := e.costAndSavings(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	return &CacheStats{
		InputTokens:       in,
		CacheCreateTokens: cc,
		CacheReadTokens:   cr,
		HitRate:           hitRate,
		NetSavingsUSD:     savings,
	}, nil
}

// Helpers ---------------------------------------------------------------

// fmtTS matches store.fmtTS — keep these in sync. SQLite-friendly ISO 8601.
func fmtTS(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.000") }

func buildTimeRange(col string, from, to time.Time) (string, []any) {
	var args []any
	clause := ""
	if !from.IsZero() && !to.IsZero() {
		clause = " WHERE " + col + " >= ? AND " + col + " < ?"
		args = []any{fmtTS(from), fmtTS(to)}
	} else if !from.IsZero() {
		clause = " WHERE " + col + " >= ?"
		args = []any{fmtTS(from)}
	} else if !to.IsZero() {
		clause = " WHERE " + col + " < ?"
		args = []any{fmtTS(to)}
	}
	return clause, args
}

func tzModifier(loc *time.Location) string {
	if loc == time.UTC {
		return "+00:00"
	}
	_, offset := time.Now().In(loc).Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	h := offset / 3600
	m := (offset % 3600) / 60
	return formatHM(sign, h, m)
}

func formatHM(sign string, h, m int) string {
	hh := []byte{'0' + byte(h/10), '0' + byte(h%10)}
	mm := []byte{'0' + byte(m/10), '0' + byte(m%10)}
	return sign + string(hh) + ":" + string(mm)
}

func firstModel(csv string) string {
	for i, c := range csv {
		if c == ',' {
			return csv[:i]
		}
	}
	return csv
}
