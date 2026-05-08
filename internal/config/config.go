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
	ClaudeDir string         `mapstructure:"claude_dir"`
	Timezone  string         `mapstructure:"timezone"`
	Server    ServerConfig   `mapstructure:"server"`
	Pricing   PricingPresets `mapstructure:"pricing"`
	Storage   StorageConfig  `mapstructure:"storage"`
	Alerts    AlertsConfig   `mapstructure:"alerts"`
}

type AlertsConfig struct {
	DailyBudgetUSD float64 `mapstructure:"daily_budget"` // 0 disables
	Notify         bool    `mapstructure:"notify"`       // macOS osascript
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type StorageConfig struct {
	Path string `mapstructure:"path"`
}

type PricingPresets struct {
	Preset   string                  `mapstructure:"preset"`
	Models   map[string]ModelPricing `mapstructure:"models"`
	Fallback ModelPricing            `mapstructure:"fallback"`
}

type ModelPricing struct {
	Input float64 `mapstructure:"input"`
	// CacheCreate is the rate (USD per 1M tokens) for the 5-minute ephemeral
	// cache write. CacheCreate1h is the rate for the 1-hour ephemeral cache
	// write — typically 1.6× higher per Anthropic's pricing. If unset, the 1h
	// rate falls back to CacheCreate (preserves backward-compatible cost).
	CacheCreate   float64 `mapstructure:"cache_create"`
	CacheCreate1h float64 `mapstructure:"cache_create_1h"`
	CacheRead     float64 `mapstructure:"cache_read"`
	Output        float64 `mapstructure:"output"`
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
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("CTL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			v.AddConfigPath(filepath.Join(home, ".config", "claude-token-lens"))
		}
	}

	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	c.ClaudeDir = expandHome(c.ClaudeDir)
	c.Storage.Path = expandHome(c.Storage.Path)
	return &c, nil
}

func setDefaults(v *viper.Viper) {
	home, _ := os.UserHomeDir()
	v.SetDefault("claude_dir", filepath.Join(home, ".claude"))
	v.SetDefault("timezone", "UTC")
	v.SetDefault("server.host", "127.0.0.1")
	v.SetDefault("server.port", 3456)
	v.SetDefault("storage.path", filepath.Join(home, ".config", "claude-token-lens", "data.db"))
	v.SetDefault("alerts.daily_budget", 0.0)
	v.SetDefault("alerts.notify", false)

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
