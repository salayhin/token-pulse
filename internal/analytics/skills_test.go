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
