package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	ClaudeDir    string             `mapstructure:"claude_dir"`
	Timezone     string             `mapstructure:"timezone"`
	Server       ServerConfig       `mapstructure:"server"`
	Pricing      PricingPresets     `mapstructure:"pricing"`
	Storage      StorageConfig      `mapstructure:"storage"`
	Alerts       AlertsConfig       `mapstructure:"alerts"`
	Subscription SubscriptionConfig `mapstructure:"subscription"`
}

// SubscriptionConfig describes how the user actually pays Anthropic. The
// per-token dollar amounts computed elsewhere in the app are *API rates*; if
// the user is on a subscription plan (Pro, Max, Team, Enterprise, etc.), those
// dollars represent "API-equivalent value" rather than real money out of pocket.
// The UI uses this struct to relabel cost figures and show a subscription-value card.
//
// Plan values:
//
//	"api"        — pay-as-you-go API, dollars are real spend (default)
//	"pro"        — Claude Pro subscription (~$20/mo, flat-fee)
//	"max_5x"     — Claude Max 5× (~$100/mo, flat-fee)
//	"max_20x"    — Claude Max 20× (~$200/mo, flat-fee)
//	"team"       — Claude Team (per-seat subscription)
//	"enterprise" — Claude Enterprise (seat price + usage at API rates)
//	"custom"     — user-defined fee (flat or variable)
type SubscriptionConfig struct {
	Plan          string  `mapstructure:"plan"            yaml:"plan"            json:"plan"`             // see comment above
	MonthlyFeeUSD float64 `mapstructure:"monthly_fee_usd" yaml:"monthly_fee_usd" json:"monthly_fee_usd"` // 0 = unset/API
}

// PlanLabel returns a human-friendly name for the configured plan.
func (s SubscriptionConfig) PlanLabel() string {
	switch s.Plan {
	case "pro":
		return "Pro"
	case "max_5x":
		return "Max 5×"
	case "max_20x":
		return "Max 20×"
	case "team":
		return "Team"
	case "enterprise":
		return "Enterprise"
	case "custom":
		return "Custom"
	default:
		return "API"
	}
}

// IsSubscription reports whether the plan is a flat-fee subscription (as
// opposed to pay-as-you-go API). The UI uses this to decide whether to
// relabel "Cost" → "API value" and surface the subscription savings card.
func (s SubscriptionConfig) IsSubscription() bool {
	switch s.Plan {
	case "pro", "max_5x", "max_20x", "team", "enterprise", "custom":
		return true
	default:
		return false
	}
}

// canonicalFees encodes the published Anthropic flat fees for plans where
// the price is fixed. Team and Custom are intentionally absent — their fees
// are user-defined (per-seat counts, custom contracts). Updating this map is
// the single point of truth for the fixed-fee plans.
var canonicalFees = map[string]float64{
	"api":     0,
	"pro":     20,
	"max_5x":  100,
	"max_20x": 200,
}

// CanonicalFee returns (fee, isFixed) for a plan. When isFixed is true the
// caller MUST use the returned fee and ignore any user input — Pro is always
// $20, Max 5× is always $100, etc. When isFixed is false (Team / Custom), the
// user-supplied fee is authoritative.
//
// This is called by the settings writer to normalize incoming PUTs and by the
// UI render path to decide whether to show the fee as an editable input vs. a
// read-only badge.
func CanonicalFee(plan string) (fee float64, isFixed bool) {
	fee, isFixed = canonicalFees[plan]
	return
}

// NormalizedFee returns the fee to actually persist for a (plan, userFee).
// Fixed plans win over user input; flexible plans pass through user input.
// Centralized so the writer, HTTP handler, and CLI all agree.
func NormalizedFee(plan string, userFee float64) float64 {
	if fee, isFixed := CanonicalFee(plan); isFixed {
		return fee
	}
	return userFee
}

type AlertsConfig struct {
	// MonthlyBudgetUSD is the single source of truth for budget alerts.
	// Daily and weekly pace are derived from it in analytics.Budget():
	//   daily_pace  = monthly / days_in_current_month
	//   weekly_pace = daily_pace * 7
	// 0 disables all budget alerts.
	MonthlyBudgetUSD float64            `mapstructure:"monthly_budget"`
	ModelBudgets     map[string]float64 `mapstructure:"model_budgets" yaml:"model_budgets" json:"model_budgets"` // per-model daily budgets
	Notify           bool               `mapstructure:"notify"`                                                 // macOS osascript
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type StorageConfig struct {
	Path string `mapstructure:"path"`
}

type PricingPresets struct {
	Preset   string                  `mapstructure:"preset"   yaml:"preset"   json:"preset"`
	Models   map[string]ModelPricing `mapstructure:"models"   yaml:"models"   json:"models"`
	Fallback ModelPricing            `mapstructure:"fallback" yaml:"fallback" json:"fallback"`
}

// ModelPricing tags must stay in sync across mapstructure (viper), yaml (writer),
// and json (settings HTTP handler). Drift causes silent round-trip data loss.
type ModelPricing struct {
	Input float64 `mapstructure:"input"          yaml:"input"          json:"input"`
	// CacheCreate is the rate (USD per 1M tokens) for the 5-minute ephemeral
	// cache write. CacheCreate1h is the rate for the 1-hour ephemeral cache
	// write — typically 1.6× higher per Anthropic's pricing. If unset, the 1h
	// rate falls back to CacheCreate (preserves backward-compatible cost).
	CacheCreate   float64 `mapstructure:"cache_create"   yaml:"cache_create"   json:"cache_create"`
	CacheCreate1h float64 `mapstructure:"cache_create_1h" yaml:"cache_create_1h" json:"cache_create_1h"`
	CacheRead     float64 `mapstructure:"cache_read"     yaml:"cache_read"     json:"cache_read"`
	Output        float64 `mapstructure:"output"         yaml:"output"         json:"output"`
}

// CacheCreate1hRate returns the 1h cache rate, falling back to CacheCreate
// when the 1h rate is unconfigured. This keeps existing user configs working.
func (p ModelPricing) CacheCreate1hRate() float64 {
	if p.CacheCreate1h > 0 {
		return p.CacheCreate1h
	}
	return p.CacheCreate
}

func (c *Config) Location() *time.Location {
	if c.Timezone == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (c *Config) PricingFor(model string) ModelPricing {
	if p, ok := c.Pricing.Models[model]; ok {
		return p
	}
	for prefix, p := range c.Pricing.Models {
		if strings.HasPrefix(model, prefix) {
			return p
		}
	}
	return c.Pricing.Fallback
}

func Load(cfgFile string) (*Config, error) {
	c, _, err := LoadWithPath(cfgFile)
	return c, err
}

// LoadWithPath loads config and also returns the resolved config file path.
func LoadWithPath(cfgFile string) (*Config, string, error) {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("TP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			v.AddConfigPath(filepath.Join(home, ".config", "tokenpulse"))
		}
	}

	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			return nil, "", fmt.Errorf("read config: %w", err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, "", fmt.Errorf("unmarshal config: %w", err)
	}
	c.ClaudeDir = expandHome(c.ClaudeDir)
	c.Storage.Path = expandHome(c.Storage.Path)
	return &c, v.ConfigFileUsed(), nil
}

// DefaultConfigPath returns the canonical write target when no config file exists.
func DefaultConfigPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "tokenpulse", "config.yaml")
	}
	return "config.yaml"
}

func setDefaults(v *viper.Viper) {
	home, _ := os.UserHomeDir()
	v.SetDefault("claude_dir", filepath.Join(home, ".claude"))
	v.SetDefault("timezone", "UTC")
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 3456)
	v.SetDefault("storage.path", filepath.Join(home, ".config", "tokenpulse", "data.db"))
	v.SetDefault("alerts.monthly_budget", 0.0)
	v.SetDefault("alerts.notify", false)

	v.SetDefault("subscription.plan", "api")
	v.SetDefault("subscription.monthly_fee_usd", 0.0)

	v.SetDefault("pricing.preset", "anthropic-api")
	// 1h cache_create rates per Anthropic API pricing (≈ 2× input rate);
	// 5m rate is ≈ 1.25× input. Verify against current published rates.
	v.SetDefault("pricing.fallback", map[string]float64{
		"input": 3.0, "output": 15.0, "cache_read": 0.30, "cache_create": 3.75, "cache_create_1h": 6.0,
	})
	v.SetDefault("pricing.models", map[string]map[string]float64{
		"claude-opus-4": {
			"input": 15.0, "output": 75.0, "cache_read": 1.50, "cache_create": 18.75, "cache_create_1h": 30.0,
		},
		"claude-sonnet-4": {
			"input": 3.0, "output": 15.0, "cache_read": 0.30, "cache_create": 3.75, "cache_create_1h": 6.0,
		},
		"claude-haiku-4": {
			"input": 1.0, "output": 5.0, "cache_read": 0.10, "cache_create": 1.25, "cache_create_1h": 2.0,
		},
	})
}

func expandHome(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
