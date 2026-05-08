package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"

	"github.com/sirajus-salayhin/claude-token-lens/internal/analytics"
	"github.com/sirajus-salayhin/claude-token-lens/internal/config"
)

// Checker reports when the day's running cost crosses the configured budget.
// It only fires once per process per crossing event.
type Checker struct {
	cfg *config.Config
	eng *analytics.Engine
	log *slog.Logger

	mu      sync.Mutex
	tripped bool
}

func New(cfg *config.Config, eng *analytics.Engine, log *slog.Logger) *Checker {
	return &Checker{cfg: cfg, eng: eng, log: log}
}

// Check runs once and notifies (terminal log + macOS notification) if today's
// cost has crossed the daily budget. Calling repeatedly is safe; it deduplicates.
func (c *Checker) Check(ctx context.Context) {
	if c.cfg.Alerts.DailyBudgetUSD <= 0 {
		return
	}
	stats, err := c.eng.Stats(ctx)
	if err != nil {
		c.log.Warn("alerts: stats failed", "err", err)
		return
	}
	cost := stats.Today.CostUSD
	if cost < c.cfg.Alerts.DailyBudgetUSD {
		// Reset trip if we somehow rolled into a new day below threshold.
		c.mu.Lock()
		c.tripped = false
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	already := c.tripped
	c.tripped = true
	c.mu.Unlock()
	if already {
		return
	}

	msg := fmt.Sprintf("Today's Claude Code spend is $%.2f (budget $%.2f).", cost, c.cfg.Alerts.DailyBudgetUSD)
	c.log.Warn("daily budget exceeded", "cost_usd", cost, "budget_usd", c.cfg.Alerts.DailyBudgetUSD)
	fmt.Fprintln(stderr(), "⚠ "+msg)
	if c.cfg.Alerts.Notify && runtime.GOOS == "darwin" {
		_ = exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification %q with title "claude-token-lens"`, msg)).Run()
	}
}
