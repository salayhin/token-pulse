package analytics

import (
	"encoding/json"
	"testing"
)

func TestExtractSkillFromToolUse(t *testing.T) {
	tests := []struct {
		name        string
		toolUseJSON string
		wantSkill   string
		wantOK      bool
	}{
		{
			name:        "skill with colon",
			toolUseJSON: `{"type":"tool_use","name":"Skill","input":{"skill":"superpowers:brainstorming"}}`,
			wantSkill:   "superpowers:brainstorming",
			wantOK:      true,
		},
		{
			name:        "slash skill",
			toolUseJSON: `{"type":"tool_use","name":"Skill","input":{"skill":"/init"}}`,
			wantSkill:   "/init",
			wantOK:      true,
		},
		{
			name:        "non-skill tool",
			toolUseJSON: `{"type":"tool_use","name":"Bash","input":{"command":"ls"}}`,
			wantSkill:   "",
			wantOK:      false,
		},
		{
			name:        "skill tool no skill param",
			toolUseJSON: `{"type":"tool_use","name":"Skill","input":{}}`,
			wantSkill:   "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var toolUse map[string]interface{}
			if err := json.Unmarshal([]byte(tt.toolUseJSON), &toolUse); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			got, ok := extractSkillFromToolUse(toolUse)
			if ok != tt.wantOK {
				t.Errorf("extractSkillFromToolUse() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantSkill {
				t.Errorf("extractSkillFromToolUse() = %q, want %q", got, tt.wantSkill)
			}
		})
	}
}

func TestParsePluginFromSkill(t *testing.T) {
	tests := []struct {
		skill     string
		wantName  string
		wantEmpty bool
	}{
		{
			skill:    "superpowers:brainstorming",
			wantName: "superpowers",
		},
		{
			skill:    "claude-api:test",
			wantName: "claude-api",
		},
		{
			skill:     "/init",
			wantEmpty: true,
		},
		{
			skill:     "standalone",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.skill, func(t *testing.T) {
			got, empty := parsePluginFromSkill(tt.skill)
			if empty != tt.wantEmpty {
				t.Errorf("parsePluginFromSkill() empty = %v, want %v", empty, tt.wantEmpty)
			}
			if !empty && got != tt.wantName {
				t.Errorf("parsePluginFromSkill() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestSkillsBreakdown_AllTime(t *testing.T) {
	// This test will be skipped until we have a mock indexer
	// For now, it documents the expected behavior
	t.Skip("requires test fixtures")

	// Expected: reads all JSONL files, extracts Skill tool params, counts, calculates %
	// Result should have non-zero TotalSkillCalls if any sessions exist
}

func TestCalculatePercentages(t *testing.T) {
	tests := []struct {
		name        string
		counts      map[string]int
		wantTotal   int
		wantPercent map[string]float64
	}{
		{
			name:        "simple",
			counts:      map[string]int{"a": 5, "b": 5},
			wantTotal:   10,
			wantPercent: map[string]float64{"a": 50.0, "b": 50.0},
		},
		{
			name:        "empty",
			counts:      map[string]int{},
			wantTotal:   0,
			wantPercent: map[string]float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentages(tt.counts)
			if result.Total != tt.wantTotal {
				t.Errorf("total = %d, want %d", result.Total, tt.wantTotal)
			}
			for name, wantPct := range tt.wantPercent {
				if got, ok := result.Percentages[name]; !ok || got != wantPct {
					t.Errorf("percentage for %q = %v, want %v", name, got, wantPct)
				}
			}
		})
	}
}

func TestSkillsBreakdown_WithMockData(t *testing.T) {
	// Create mock JSONL data
	mockRecord := map[string]interface{}{
		"type": "assistant",
		"message": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"name": "Skill",
					"input": map[string]interface{}{
						"skill": "superpowers:brainstorming",
					},
				},
			},
		},
	}

	data, _ := json.Marshal(mockRecord)
	line := string(data)

	// Verify parsing works
	var record map[string]interface{}
	json.Unmarshal([]byte(line), &record)

	msg := record["message"].(map[string]interface{})
	content := msg["content"].([]interface{})
	toolUse := content[0].(map[string]interface{})

	skill, ok := extractSkillFromToolUse(toolUse)
	if !ok || skill != "superpowers:brainstorming" {
		t.Errorf("failed to extract skill from mock data")
	}
}
