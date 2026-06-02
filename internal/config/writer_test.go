package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteSettings_PreservesUnrelatedSubtrees verifies the writer only
// touches `timezone` and `pricing`, leaving the rest of the user's config
// (server, storage, alerts) byte-for-byte intact.
func TestWriteSettings_PreservesUnrelatedSubtrees(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	const original = `claude_dir: "~/.claude"
timezone: "UTC"

server:
  host: "127.0.0.1"
  port: 3456

storage:
  path: "~/.config/claude-token-lens/data.db"

alerts:
  daily_budget: 25.0
  notify: true

# USD per 1M tokens.
pricing:
  preset: "anthropic-api"
  fallback:
    input: 3.0
    output: 15.0
    cache_read: 0.30
    cache_create: 3.75
    cache_create_1h: 6.0
  models:
    claude-sonnet-4:
      input: 3.0
      output: 15.0
      cache_read: 0.30
      cache_create: 3.75
      cache_create_1h: 6.0
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	patch := SettingsPatch{
		Timezone: "Asia/Tokyo",
		Pricing: PricingPresets{
			Preset: "anthropic-api",
			Fallback: ModelPricing{
				Input: 3.0, Output: 15.0, CacheRead: 0.30,
				CacheCreate: 3.75, CacheCreate1h: 6.0,
			},
			Models: map[string]ModelPricing{
				"claude-sonnet-4": {
					Input: 3.5, Output: 18.0, CacheRead: 0.35,
					CacheCreate: 4.0, CacheCreate1h: 7.0,
				},
			},
		},
	}
	if err := WriteSettings(path, patch); err != nil {
		t.Fatalf("WriteSettings: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	s := string(got)

	// Unrelated subtrees must survive verbatim.
	for _, want := range []string{
		`host: "127.0.0.1"`,
		"port: 3456",
		`path: "~/.config/claude-token-lens/data.db"`,
		"daily_budget: 25",
		"notify: true",
		`claude_dir: "~/.claude"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n--- file ---\n%s", want, s)
		}
	}

	// New pricing must be written.
	if !strings.Contains(s, "Asia/Tokyo") {
		t.Errorf("timezone not updated:\n%s", s)
	}
	if !strings.Contains(s, "input: 3.5") || !strings.Contains(s, "output: 18") {
		t.Errorf("sonnet rates not updated:\n%s", s)
	}

	// Reload via Load and confirm Config matches.
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Timezone != "Asia/Tokyo" {
		t.Errorf("timezone after reload: %q", c.Timezone)
	}
	mp, ok := c.Pricing.Models["claude-sonnet-4"]
	if !ok {
		t.Fatalf("sonnet missing after reload; models=%v", c.Pricing.Models)
	}
	if mp.Input != 3.5 || mp.CacheCreate1h != 7.0 {
		t.Errorf("sonnet rates wrong after reload: %+v", mp)
	}
	// Untouched subtree round-trips.
	if c.Server.Port != 3456 || c.Server.Host != "127.0.0.1" {
		t.Errorf("server config drifted: %+v", c.Server)
	}
	if c.Alerts.DailyBudgetUSD != 25.0 || !c.Alerts.Notify {
		t.Errorf("alerts drifted: %+v", c.Alerts)
	}
}

// TestWriteSettings_CreatesFileWhenAbsent covers the first-save path where
// no config.yaml exists yet on disk.
func TestWriteSettings_CreatesFileWhenAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	patch := SettingsPatch{
		Timezone: "UTC",
		Pricing: PricingPresets{
			Preset: "anthropic-api",
			Fallback: ModelPricing{
				Input: 3.0, Output: 15.0, CacheRead: 0.30,
				CacheCreate: 3.75, CacheCreate1h: 6.0,
			},
		},
	}
	if err := WriteSettings(path, patch); err != nil {
		t.Fatalf("WriteSettings: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Pricing.Fallback.Input != 3.0 {
		t.Errorf("fallback rate not persisted: %+v", c.Pricing.Fallback)
	}
}

func TestValidateSettingsPatch(t *testing.T) {
	t.Parallel()
	good := SettingsPatch{
		Timezone: "UTC",
		Pricing: PricingPresets{
			Fallback: ModelPricing{Input: 3.0, Output: 15.0, CacheRead: 0.3, CacheCreate: 3.75, CacheCreate1h: 6.0},
		},
	}
	if err := ValidateSettingsPatch(good); err != nil {
		t.Fatalf("good patch rejected: %v", err)
	}

	cases := []struct {
		name  string
		patch SettingsPatch
	}{
		{"empty tz", SettingsPatch{Timezone: "", Pricing: good.Pricing}},
		{"bad tz", SettingsPatch{Timezone: "Not/A/Zone", Pricing: good.Pricing}},
		{"negative rate", func() SettingsPatch {
			p := good
			p.Pricing.Fallback.Input = -1
			return p
		}()},
		{"empty model name", func() SettingsPatch {
			p := good
			p.Pricing.Models = map[string]ModelPricing{"  ": good.Pricing.Fallback}
			return p
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateSettingsPatch(c.patch); err == nil {
				t.Errorf("expected error for %s", c.name)
			}
		})
	}
}
