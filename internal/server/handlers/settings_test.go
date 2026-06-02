package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirajus-salayhin/tokenpulse/internal/config"
)

// newTestSettings builds an isolated Provider + Handler pair backed by a
// tempfile so each test owns its own config.yaml.
func newTestSettings(t *testing.T) (*SettingsHandler, *config.Provider, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := &config.Config{
		Timezone: "UTC",
		Pricing: config.PricingPresets{
			Preset: "anthropic-api",
			Fallback: config.ModelPricing{
				Input: 3, Output: 15, CacheRead: 0.30,
				CacheCreate: 3.75, CacheCreate1h: 6.0,
			},
			Models: map[string]config.ModelPricing{
				"claude-sonnet-4": {
					Input: 3, Output: 15, CacheRead: 0.30,
					CacheCreate: 3.75, CacheCreate1h: 6.0,
				},
			},
		},
	}
	p := config.NewProvider(cfg)
	return NewSettingsHandler(p, path, stubObserved{"claude-sonnet-4-5-20250929", "claude-opus-4-1-20250805"}), p, path
}

// stubObserved is a tiny fake that returns a fixed list of "observed" model
// ids, so settings handler tests don't need a real database.
type stubObserved []string

func (s stubObserved) ObservedModels(_ context.Context) ([]string, error) {
	return []string(s), nil
}

func TestSettings_GetReturnsCurrent(t *testing.T) {
	h, _, _ := newTestSettings(t)
	rr := httptest.NewRecorder()
	h.Get(rr, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["timezone"] != "UTC" {
		t.Errorf("timezone=%v", resp["timezone"])
	}
	if _, ok := resp["pricing"]; !ok {
		t.Errorf("pricing missing: %v", resp)
	}
	// The settings page renders one row per observed model, so the GET
	// must surface them. Stub returns two; assert both come through.
	observed, ok := resp["observed_models"].([]any)
	if !ok || len(observed) != 2 {
		t.Errorf("observed_models missing or wrong length: %v", resp["observed_models"])
	}
}

func TestSettings_PutPersistsAndSwapsProvider(t *testing.T) {
	h, p, path := newTestSettings(t)

	body := []byte(`{
		"timezone": "Asia/Tokyo",
		"pricing": {
			"preset": "anthropic-api",
			"fallback": {"input":3,"output":15,"cache_read":0.30,"cache_create":3.75,"cache_create_1h":6.0},
			"models": {
				"claude-sonnet-4": {"input":3.5,"output":18.0,"cache_read":0.35,"cache_create":4.0,"cache_create_1h":7.0}
			}
		}
	}`)
	rr := httptest.NewRecorder()
	h.Put(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Provider should now reflect new values without re-reading the file.
	got := p.Get()
	if got.Timezone != "Asia/Tokyo" {
		t.Errorf("provider timezone=%q", got.Timezone)
	}
	if got.Pricing.Models["claude-sonnet-4"].Input != 3.5 {
		t.Errorf("provider sonnet=%+v", got.Pricing.Models["claude-sonnet-4"])
	}

	// And the file on disk must round-trip cleanly through Load.
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Timezone != "Asia/Tokyo" {
		t.Errorf("file timezone=%q", loaded.Timezone)
	}
	if loaded.Pricing.Models["claude-sonnet-4"].CacheCreate1h != 7.0 {
		t.Errorf("file 1h rate=%+v", loaded.Pricing.Models["claude-sonnet-4"])
	}
}

func TestSettings_PutRejectsBadTimezone(t *testing.T) {
	h, _, path := newTestSettings(t)
	body := []byte(`{"timezone":"Not/A/Zone","pricing":{"fallback":{"input":3,"output":15,"cache_read":0.3,"cache_create":3.75,"cache_create_1h":6}}}`)
	rr := httptest.NewRecorder()
	h.Put(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid timezone") {
		t.Errorf("expected validation error in body, got: %s", rr.Body.String())
	}
	// File must not have been written.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should not exist after failed PUT; stat err=%v", err)
	}
}

func TestSettings_PutRejectsNegativeRate(t *testing.T) {
	h, _, _ := newTestSettings(t)
	body := []byte(`{"timezone":"UTC","pricing":{"fallback":{"input":-1,"output":15,"cache_read":0.3,"cache_create":3.75,"cache_create_1h":6}}}`)
	rr := httptest.NewRecorder()
	h.Put(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(body)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSettings_PutRejectsMalformedJSON(t *testing.T) {
	h, _, _ := newTestSettings(t)
	rr := httptest.NewRecorder()
	h.Put(rr, httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader("{not json")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
