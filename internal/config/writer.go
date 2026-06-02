package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// SettingsPatch is the subset of Config that the Settings UI is allowed to
// mutate. Anything else in config.yaml (server host/port, claude_dir, storage
// path) is preserved verbatim — including comments and key ordering. The alerts
// section is merged: monthly_budget and model_budgets are updated; notify is
// preserved.
//
// MonthlyBudgetUSD uses a pointer type so the JSON/YAML wire format can
// distinguish "field omitted" (leave unchanged) from "explicitly set to 0"
// (disable the alert). The HTTP layer always sends the field so in practice
// a nil pointer only appears in programmatic patches.
type SettingsPatch struct {
	Timezone         string              `yaml:"timezone"`
	Pricing          PricingPresets      `yaml:"pricing"`
	ModelBudgets     map[string]float64  `yaml:"model_budgets"`
	MonthlyBudgetUSD *float64            `yaml:"monthly_budget,omitempty"`
	Subscription     *SubscriptionConfig `yaml:"subscription,omitempty"`
}

// ValidateSettingsPatch is shared between the writer and the HTTP handler so
// the same rules apply to disk and to API input. Returns the first error.
func ValidateSettingsPatch(p SettingsPatch) error {
	if p.Timezone == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", p.Timezone, err)
	}
	if err := validatePricing(p.Pricing.Fallback, "fallback"); err != nil {
		return err
	}
	for name, mp := range p.Pricing.Models {
		if strings.TrimSpace(name) == "" {
			return errors.New("model name cannot be empty")
		}
		if err := validatePricing(mp, name); err != nil {
			return err
		}
	}
	for name, budget := range p.ModelBudgets {
		if strings.TrimSpace(name) == "" {
			return errors.New("model budget name cannot be empty")
		}
		if budget < 0 {
			return fmt.Errorf("model budget for %q must be ≥ 0", name)
		}
		if math.IsNaN(budget) || math.IsInf(budget, 0) {
			return fmt.Errorf("model budget for %q must be a finite number", name)
		}
	}
	if p.MonthlyBudgetUSD != nil {
		v := *p.MonthlyBudgetUSD
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("monthly_budget must be a finite number")
		}
		if v < 0 {
			return errors.New("monthly_budget must be ≥ 0")
		}
	}
	if p.Subscription != nil {
		if !validPlans[p.Subscription.Plan] {
			return fmt.Errorf("subscription.plan must be one of api, pro, max_5x, max_20x, team, custom (got %q)", p.Subscription.Plan)
		}
		v := p.Subscription.MonthlyFeeUSD
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("subscription.monthly_fee_usd must be a finite number")
		}
		if v < 0 {
			return errors.New("subscription.monthly_fee_usd must be ≥ 0")
		}
	}
	return nil
}

// validPlans guards SubscriptionConfig.Plan at the validation layer so
// unknown values can't enter the YAML — keeps the read-side switch in
// SubscriptionConfig.PlanLabel honest about the universe of inputs.
var validPlans = map[string]bool{
	"api": true, "pro": true, "max_5x": true, "max_20x": true,
	"team": true, "custom": true,
}

func validatePricing(p ModelPricing, label string) error {
	check := func(field string, v float64) error {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s.%s must be a finite number", label, field)
		}
		if v < 0 {
			return fmt.Errorf("%s.%s must be ≥ 0", label, field)
		}
		return nil
	}
	if err := check("input", p.Input); err != nil {
		return err
	}
	if err := check("output", p.Output); err != nil {
		return err
	}
	if err := check("cache_read", p.CacheRead); err != nil {
		return err
	}
	if err := check("cache_create", p.CacheCreate); err != nil {
		return err
	}
	if err := check("cache_create_1h", p.CacheCreate1h); err != nil {
		return err
	}
	return nil
}

// WriteSettings persists a SettingsPatch to the YAML file at path. It uses
// yaml.v3's Node API so that:
//   - Existing top-level keys (server, storage, alerts, claude_dir) are preserved
//     verbatim, including their comments and ordering.
//   - The `timezone` and `pricing` subtrees are replaced wholesale.
//   - Comments attached to the top-level `pricing` or `timezone` keys are kept.
//
// If the file does not exist, a fresh document is created from the patch alone.
// Writes are atomic via tmp-file + rename.
func WriteSettings(path string, patch SettingsPatch) error {
	if err := ValidateSettingsPatch(patch); err != nil {
		return err
	}

	var root yaml.Node
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Fresh document — synthesize a doc node with mapping content.
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
			{Kind: yaml.MappingNode},
		}}
	case err != nil:
		return fmt.Errorf("read %s: %w", path, err)
	default:
		if err := yaml.Unmarshal(existing, &root); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		// Empty files unmarshal to a zero-valued node; treat like fresh.
		if root.Kind == 0 {
			root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
				{Kind: yaml.MappingNode},
			}}
		}
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("unexpected yaml structure in %s", path)
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("top-level of %s is not a mapping", path)
	}

	tzNode, err := nodeFromScalar(patch.Timezone)
	if err != nil {
		return err
	}
	setOrInsert(mapping, "timezone", tzNode)

	priceNode, err := nodeFromValue(patch.Pricing)
	if err != nil {
		return err
	}
	setOrInsert(mapping, "pricing", priceNode)

	if patch.Subscription != nil {
		subNode, err := nodeFromValue(*patch.Subscription)
		if err != nil {
			return err
		}
		setOrInsert(mapping, "subscription", subNode)
	}

	// For alerts, merge budgets into the existing alerts subtree, preserving
	// any keys we don't touch (notably notify).
	alertsNode := getOrInsert(mapping, "alerts")
	if alertsNode.Kind == yaml.MappingNode {
		mbNode, err := nodeFromValue(patch.ModelBudgets)
		if err != nil {
			return err
		}
		setOrInsert(alertsNode, "model_budgets", mbNode)
		if patch.MonthlyBudgetUSD != nil {
			n, err := nodeFromValue(*patch.MonthlyBudgetUSD)
			if err != nil {
				return err
			}
			setOrInsert(alertsNode, "monthly_budget", n)
		}
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// getOrInsert returns the value node for `key` in a mapping, creating an
// empty mapping node if the key doesn't exist.
func getOrInsert(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Value == key {
			return mapping.Content[i+1]
		}
	}
	// Key not found; create an empty mapping and insert it.
	newNode := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		newNode,
	)
	return newNode
}

// setOrInsert replaces the value for `key` in a mapping node, preserving the
// key node itself (and thus any HeadComment / LineComment attached to it).
// If the key is absent, it's appended at the end.
func setOrInsert(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Value == key {
			// Preserve the key node (with its comments). Replace value only.
			// Also clear LineComment on the value side that may have been
			// attached to the old value.
			value.HeadComment = mapping.Content[i+1].HeadComment
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		value,
	)
}

func nodeFromScalar(s string) (*yaml.Node, error) {
	n := &yaml.Node{Kind: yaml.ScalarNode, Value: s, Tag: "!!str", Style: yaml.DoubleQuotedStyle}
	return n, nil
}

// nodeFromValue round-trips a Go value through yaml.Marshal → Unmarshal back
// into a Node so we get a properly typed YAML subtree without hand-building it.
func nodeFromValue(v any) (*yaml.Node, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) == 0 {
		return nil, fmt.Errorf("unexpected node from marshal: kind=%d", n.Kind)
	}
	return n.Content[0], nil
}
