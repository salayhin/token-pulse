package analytics

import "github.com/sirajus-salayhin/claude-token-lens/internal/config"

const tokensPerMillion = 1_000_000.0

// splitCacheCreate resolves an effective (5m, 1h) split from the three columns
// stored per message. cLegacy is the wire-format sum (always populated);
// c5m/c1h carry the split (populated only after the schema migration). Any
// portion of the legacy sum not accounted for by 5m+1h is treated as 5m —
// preserving pre-migration billing where everything was charged at the 5m rate.
func splitCacheCreate(c5m, c1h, cLegacy int) (eff5m, eff1h int) {
	unallocated := cLegacy - c5m - c1h
	if unallocated < 0 {
		unallocated = 0
	}
	return c5m + unallocated, c1h
}

// CostUSD computes the cost in USD for a given token mix and model pricing.
// Pricing rates are USD per 1M tokens. The cache-creation columns are passed
// as (5m, 1h, legacy) — see splitCacheCreate for the fallback rule.
func CostUSD(p config.ModelPricing, inputTok, outputTok, cacheCreate5m, cacheCreate1h, cacheCreateLegacy, cacheReadTok int) float64 {
	eff5m, eff1h := splitCacheCreate(cacheCreate5m, cacheCreate1h, cacheCreateLegacy)
	return float64(inputTok)*p.Input/tokensPerMillion +
		float64(outputTok)*p.Output/tokensPerMillion +
		float64(eff5m)*p.CacheCreate/tokensPerMillion +
		float64(eff1h)*p.CacheCreate1hRate()/tokensPerMillion +
		float64(cacheReadTok)*p.CacheRead/tokensPerMillion
}

// NetCacheSavingsUSD returns the realized savings from cache usage vs a
// counterfactual where every cache_read had been a fresh input AND we had
// not paid the cache-creation premium. The 5m and 1h ephemeral cache writes
// have different premiums, so they're charged separately:
//
//	gross_saved   = cache_read_tokens × (input_rate − cache_read_rate)
//	extra_paid_5m = cache_create_5m   × (cache_create_5m_rate − input_rate)
//	extra_paid_1h = cache_create_1h   × (cache_create_1h_rate − input_rate)
//	net_saved_usd = gross_saved − extra_paid_5m − extra_paid_1h
func NetCacheSavingsUSD(p config.ModelPricing, cacheCreate5m, cacheCreate1h, cacheCreateLegacy, cacheReadTok int) float64 {
	eff5m, eff1h := splitCacheCreate(cacheCreate5m, cacheCreate1h, cacheCreateLegacy)
	grossSaved := float64(cacheReadTok) * (p.Input - p.CacheRead) / tokensPerMillion
	extraPaid5m := float64(eff5m) * (p.CacheCreate - p.Input) / tokensPerMillion
	extraPaid1h := float64(eff1h) * (p.CacheCreate1hRate() - p.Input) / tokensPerMillion
	return grossSaved - extraPaid5m - extraPaid1h
}
