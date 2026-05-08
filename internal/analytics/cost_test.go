package analytics

import (
	"math"
	"testing"

	"github.com/sirajus-salayhin/claude-token-lens/internal/config"
)

func approxEq(a, b, eps float64) bool { return math.Abs(a-b) < eps }

func TestCostUSD(t *testing.T) {
	p := config.ModelPricing{Input: 3.0, Output: 15.0, CacheCreate: 3.75, CacheRead: 0.30}
	// 1M input × $3 + 1M output × $15 = $18
	got := CostUSD(p, 1_000_000, 1_000_000, 0, 0)
	if !approxEq(got, 18.0, 1e-9) {
		t.Errorf("got %f, want 18.0", got)
	}
	// 100k cache_create × $3.75/M + 1M cache_read × $0.30/M = 0.375 + 0.30 = 0.675
	got = CostUSD(p, 0, 0, 100_000, 1_000_000)
	if !approxEq(got, 0.675, 1e-9) {
		t.Errorf("got %f, want 0.675", got)
	}
}

func TestNetCacheSavingsUSD(t *testing.T) {
	p := config.ModelPricing{Input: 3.0, Output: 15.0, CacheCreate: 3.75, CacheRead: 0.30}
	// 1M cache_read: gross_saved = 1M × (3.0 - 0.30) / 1M = $2.70
	// 1M cache_create: extra_paid = 1M × (3.75 - 3.0) / 1M = $0.75
	// net = 2.70 - 0.75 = $1.95
	got := NetCacheSavingsUSD(p, 1_000_000, 1_000_000)
	if !approxEq(got, 1.95, 1e-9) {
		t.Errorf("got %f, want 1.95", got)
	}

	// All read, no create: pure savings.
	got = NetCacheSavingsUSD(p, 0, 1_000_000)
	if !approxEq(got, 2.70, 1e-9) {
		t.Errorf("read-only savings: got %f, want 2.70", got)
	}

	// All create, no read: pure premium loss.
	got = NetCacheSavingsUSD(p, 1_000_000, 0)
	if !approxEq(got, -0.75, 1e-9) {
		t.Errorf("create-only loss: got %f, want -0.75", got)
	}
}
