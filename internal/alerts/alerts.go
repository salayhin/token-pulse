package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"

	"github.com/sirajus-salayhin/tokenpulse/internal/analytics"
	"github.com/sirajus-salayhin/tokenpulse/internal/config"
)

// Checker reports when month-to-date spend crosses the configured monthly
// budget, or — as an early warning — when the daily pace is exceeded (today's
// spend > monthly / days_in_month). Each trip fires once per crossing event;
// the daily trip resets at midnight, the monthly trip resets on the 1st.
//
// The Checker reads the live config through a *config.Provider so settings
// changes (e.g. raising or removing the monthly budget) take effect on the
// next Check() call without a restart.
type Checker struct {
	provider *config.Provider
	eng      *analytics.Engine
	log      *slog.Logger

	mu             sync.Mutex
	trippedDaily   bool
	trippedMonthly bool
	trippedByModel map[string]bool // tracks which models have exceeded their budgets
}

func New(provider *config.Provider, eng *analytics.Engine, log *slog.Logger) *Checker {
	return &Checker{
		provider:       provider,
		eng:            eng,
		log:            log,
		trippedByModel: make(map[string]bool),
	}
}

// NewWithConfig wraps a static *Config in a one-shot provider so CLI
// subcommands can still construct a Checker without plumbing a provider.
func NewWithConfig(cfg *config.Config, eng *analytics.Engine, log *slog.Logger) *Checker {
	return New(config.NewProvider(cfg), eng, log)
}

// cfg returns the current config snapshot. Cheap (atomic.Pointer load).
func (c *Checker) cfg() *config.Config { return c.provider.Get() }

// Check runs once and notifies (terminal log + macOS notification) if today's
// derived-daily pace or month-to-date budget has been exceeded. Calling
// repeatedly is safe; it deduplicates per-period. Per-model budget checking is
// a future extension and currently a no-op.
func (c *Checker) Check(ctx context.Context) {
	cfg := c.cfg()
	hasMonthly := cfg.Alerts.MonthlyBudgetUSD > 0
	hasModel := len(cfg.Alerts.ModelBudgets) > 0
	if !hasMonthly && !hasModel {
		return
	}

	if hasMonthly {
		b, err := c.eng.Budget(ctx)
		if err != nil {
			c.log.Warn("alerts: budget failed", "err", err)
		} else {
			c.checkDailyPace(b)
			c.checkMonthlyBudget(b)
		}
	}
	// Per-model budget checking would require additional analytics queries.
	// Stub for future implementation: c.checkModelBudgets(ctx)
}

func (c *Checker) checkDailyPace(b *analytics.BudgetResponse) {
	if b.DerivedDaily <= 0 {
		return
	}
	cost := b.Day.CostUSD
	if cost < b.DerivedDaily {
		// Reset trip if we somehow rolled into a new day below threshold.
		c.mu.Lock()
		c.trippedDaily = false
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	already := c.trippedDaily
	c.trippedDaily = true
	c.mu.Unlock()
	if already {
		return
	}

	msg := fmt.Sprintf("Today's spend ($%.2f) exceeds the daily pace ($%.2f from $%.2f/mo).",
		cost, b.DerivedDaily, b.MonthlyBudget)
	c.log.Warn("daily pace exceeded", "cost_usd", cost, "daily_pace_usd", b.DerivedDaily)
	fmt.Fprintln(stderr(), "⚠ "+msg)
	if c.cfg().Alerts.Notify && runtime.GOOS == "darwin" {
		_ = exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification %q with title "tokenpulse"`, msg)).Run()
	}
}

func (c *Checker) checkMonthlyBudget(b *analytics.BudgetResponse) {
	cfg := c.cfg()
	if cfg.Alerts.MonthlyBudgetUSD <= 0 {
		return
	}
	cost := b.Month.CostUSD
	if cost < cfg.Alerts.MonthlyBudgetUSD {
		c.mu.Lock()
		c.trippedMonthly = false
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	already := c.trippedMonthly
	c.trippedMonthly = true
	c.mu.Unlock()
	if already {
		return
	}

	msg := fmt.Sprintf("This month's Claude Code spend is $%.2f (budget $%.2f).",
		cost, cfg.Alerts.MonthlyBudgetUSD)
	c.log.Warn("monthly budget exceeded", "cost_usd", cost, "budget_usd", cfg.Alerts.MonthlyBudgetUSD)
	fmt.Fprintln(stderr(), "⚠ "+msg)
	if cfg.Alerts.Notify && runtime.GOOS == "darwin" {
		_ = exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification %q with title "tokenpulse"`, msg)).Run()
	}
}
