package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/sirajus-salayhin/tokenpulse/internal/config"
)

// observedModelsSource lets the settings handler look up which model ids are
// actually present in the user's data. Wired to analytics.Engine.ObservedModels
// in production; tests can pass a stub.
type observedModelsSource interface {
	ObservedModels(ctx context.Context) ([]string, error)
}

// SettingsHandler owns the read/write path for /api/v1/settings. The mutex
// serializes concurrent PUTs so the YAML file is never written by two
// goroutines at once; reads remain lock-free (atomic.Pointer in Provider).
type SettingsHandler struct {
	provider *config.Provider
	observed observedModelsSource
	// configPath is the path WriteSettings writes to. Captured once at
	// startup from viper's resolved file; falls back to the default
	// ~/.config path on first save.
	configPath string

	mu sync.Mutex
}

func NewSettingsHandler(provider *config.Provider, configPath string, observed observedModelsSource) *SettingsHandler {
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}
	return &SettingsHandler{provider: provider, observed: observed, configPath: configPath}
}

// settingsResponse is the JSON shape returned by GET and accepted by PUT.
// observed_models is GET-only — included so the UI can render one row per
// real model the user is actually billed for, instead of the abstract
// "fallback" pseudo-row.
type settingsResponse struct {
	Timezone       string                `json:"timezone"`
	Pricing        config.PricingPresets `json:"pricing"`
	ModelBudgets   map[string]float64    `json:"model_budgets"`
	ObservedModels []string              `json:"observed_models,omitempty"`
	ConfigPath     string                `json:"config_path,omitempty"`
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := h.provider.Get()
	var observed []string
	if h.observed != nil {
		// A DB hiccup here shouldn't blow up the whole settings page —
		// degrade gracefully to "no observed models" and let the UI fall
		// back to whatever's in config.yaml.
		if got, err := h.observed.ObservedModels(r.Context()); err == nil {
			observed = got
		}
	}
	modelBudgets := cfg.Alerts.ModelBudgets
	if modelBudgets == nil {
		modelBudgets = make(map[string]float64)
	}
	resp := settingsResponse{
		Timezone:       cfg.Timezone,
		Pricing:        cfg.Pricing,
		ModelBudgets:   modelBudgets,
		ObservedModels: observed,
		ConfigPath:     h.configPath,
	}
	writeJSON(w, resp)
}

func (h *SettingsHandler) Put(w http.ResponseWriter, r *http.Request) {
	var in settingsResponse
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	patch := config.SettingsPatch{
		Timezone:     in.Timezone,
		Pricing:      in.Pricing,
		ModelBudgets: in.ModelBudgets,
	}
	if err := config.ValidateSettingsPatch(patch); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if err := config.WriteSettings(h.configPath, patch); err != nil {
		writeErr(w, http.StatusInternalServerError, "write config: "+err.Error())
		return
	}
	// Re-read so env-var overrides and viper's normalization both apply
	// consistently to the in-memory snapshot.
	newCfg, _, err := config.LoadWithPath(h.configPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "reload config: "+err.Error())
		return
	}
	h.provider.Set(newCfg)

	// Mirror GET's shape so the UI can re-render without a follow-up fetch.
	var observed []string
	if h.observed != nil {
		if got, err := h.observed.ObservedModels(r.Context()); err == nil {
			observed = got
		}
	}
	modelBudgets := newCfg.Alerts.ModelBudgets
	if modelBudgets == nil {
		modelBudgets = make(map[string]float64)
	}
	writeJSON(w, settingsResponse{
		Timezone:       newCfg.Timezone,
		Pricing:        newCfg.Pricing,
		ModelBudgets:   modelBudgets,
		ObservedModels: observed,
		ConfigPath:     h.configPath,
	})
}
