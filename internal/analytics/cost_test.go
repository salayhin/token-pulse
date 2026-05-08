package analytics

import (
	"math"
	"testing"

	"github.com/sirajus-salayhin/claude-token-lens/internal/config"
)

func approxEq(a, b, eps float64) bool { return math.Abs(a-b) < eps }

func TestCostUSD(t *testing.T) {
	p := config.ModelPricing{Input: 3.0, Output: 15.0, CacheCreate: 3.75, CacheCreate1h: 6.0, CacheRead: 0.30}

	// Pure input/output: 1M input × $3 + 1M output × $15 = $18.
	got := CostUSD(p, 1_000_000, 1_000_000, 0, 0, 0, 0)
	if !approxEq(got, 18.0, 1e-9) {
		t.Errorf("input+output: got %f, want 18.0", got)
	}

	// 100k 5m cache_create × $3.75/M + 1M cache_read × $0.30/M = 0.375 + 0.30 = 0.675.
	// Caller passes legacy = 5m (matches new wire format).
	got = CostUSD(p, 0, 0, 100_000, 0, 100_000, 1_000_000)
	if !approxEq(got, 0.675, 1e-9) {
		t.Errorf("5m cache_create + cache_read: got %f, want 0.675", got)
	}

	// 100k 1h cache_create × $6.00/M = $0.60 (1h rate, not 5m).
	got = CostUSD(p, 0, 0, 0, 100_000, 100_000, 0)
	if !approxEq(got, 0.60, 1e-9) {
		t.Errorf("1h cache_create: got %f, want 0.60", got)
	}

	// Backward-compat: legacy column populated, split columns zero.
	// Falls back to billing the legacy total entirely at the 5m rate.
	got = CostUSD(p, 0, 0, 0, 0, 100_000, 0)
	if !approxEq(got, 0.375, 1e-9) {
		t.Errorf("legacy fallback (5m rate): got %f, want 0.375", got)
	}
}

func TestNetCacheSavingsUSD(t *testing.T) {
	p := config.ModelPricing{Input: 3.0, Output: 15.0, CacheCreate: 3.75, CacheCreate1h: 6.0, CacheRead: 0.30}

	// 1M cache_read: gross_saved = 1M × (3.0 - 0.30) / 1M = $2.70.
	// 1M 5m cache_create: extra_paid_5m = 1M × (3.75 - 3.0) / 1M = $0.75.
	// net = 2.70 - 0.75 = $1.95.
	got := NetCacheSavingsUSD(p, 1_000_000, 0, 1_000_000, 1_000_000)
	if !approxEq(got, 1.95, 1e-9) {
		t.Errorf("5m create + read: got %f, want 1.95", got)
	}

	// 1M 1h cache_create: extra_paid_1h = 1M × (6.0 - 3.0) / 1M = $3.00.
	// 1M cache_read: gross_saved = $2.70.
	// net = 2.70 - 3.00 = -$0.30 (1h cache makes savings net-negative here).
	got = NetCacheSavingsUSD(p, 0, 1_000_000, 1_000_000, 1_000_000)
	if !approxEq(got, -0.30, 1e-9) {
		t.Errorf("1h create + read: got %f, want -0.30", got)
	}

	// All read, no create: pure savings.
	got = NetCacheSavingsUSD(p, 0, 0, 0, 1_000_000)
	if !approxEq(got, 2.70, 1e-9) {
		t.Errorf("read-only savings: got %f, want 2.70", got)
	}

	// All 5m create, no read: pure premium loss.
	got = NetCacheSavingsUSD(p, 1_000_000, 0, 1_000_000, 0)
	if !approxEq(got, -0.75, 1e-9) {
		t.Errorf("create-only loss (5m): got %f, want -0.75", got)
	}

	// Backward-compat: legacy column populated, split columns zero.
	// Treats legacy entirely as 5m.
	got = NetCacheSavingsUSD(p, 0, 0, 1_000_000, 0)
	if !approxEq(got, -0.75, 1e-9) {
		t.Errorf("legacy fallback (5m loss): got %f, want -0.75", got)
	}
}

func TestSplitCacheCreate(t *testing.T) {
	// Pure new-format: 5m=80, 1h=20, legacy=100. Effective is identity.
	e5m, e1h := splitCacheCreate(80, 20, 100)
	if e5m != 80 || e1h != 20 {
		t.Errorf("new format: got (%d, %d), want (80, 20)", e5m, e1h)
	}

	// Pure pre-migration: split columns zero, legacy nonzero. Everything goes to 5m.
	e5m, e1h = splitCacheCreate(0, 0, 100)
	if e5m != 100 || e1h != 0 {
		t.Errorf("pre-migration: got (%d, %d), want (100, 0)", e5m, e1h)
	}

	// Mixed (aggregate over old + new rows): legacy=100, but split sums to 60.
	// The unallocated 40 is treated as 5m.
	e5m, e1h = splitCacheCreate(40, 20, 100)
	if e5m != 80 || e1h != 20 {
		t.Errorf("mixed aggregate: got (%d, %d), want (80, 20)", e5m, e1h)
	}

	// Defensive: split sums exceed legacy (shouldn't happen, but no negative).
	e5m, e1h = splitCacheCreate(80, 30, 100)
	if e5m != 80 || e1h != 30 {
		t.Errorf("split>legacy: got (%d, %d), want (80, 30)", e5m, e1h)
	}
}
