package analytics

import "github.com/sirajus-salayhin/claude-token-lens/internal/config"

// CostUSD computes the cost in USD for a given token mix and model pricing.
// Pricing rates are USD per 1M tokens.
func CostUSD(p config.ModelPricing, inputTok, outputTok, cacheCreateTok, cacheReadTok int) float64 {
	const M = 1_000_000.0
	return float64(inputTok)*p.Input/M +
		float64(outputTok)*p.Output/M +
		float64(cacheCreateTok)*p.CacheCreate/M +
		float64(cacheReadTok)*p.CacheRead/M
}

// NetCacheSavingsUSD returns the realized savings from cache usage,
// vs a counterfactual where every cache_read had been a fresh input
// AND we had not paid the cache-creation premium.
//
//	gross_saved   = cache_read_tokens   × (input_rate − cache_read_rate)
//	extra_paid    = cache_create_tokens × (cache_create_rate − input_rate)
//	net_saved_usd = gross_saved − extra_paid
//
// Per-1M rates are converted internally.
func NetCacheSavingsUSD(p config.ModelPricing, cacheCreateTok, cacheReadTok int) float64 {
	const M = 1_000_000.0
	grossSaved := float64(cacheReadTok) * (p.Input - p.CacheRead) / M
	extraPaid := float64(cacheCreateTok) * (p.CacheCreate - p.Input) / M
	return grossSaved - extraPaid
}
