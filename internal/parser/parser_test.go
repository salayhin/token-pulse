package parser

import (
	"strings"
	"testing"
)

const sampleJSONL = `{"type":"permission-mode","permissionMode":"default","sessionId":"sess-1"}
{"parentUuid":null,"type":"user","message":{"role":"user","content":"hello"},"uuid":"u-1","timestamp":"2026-04-14T08:03:50.311Z","cwd":"/p","sessionId":"sess-1","version":"2.1.105","gitBranch":"main"}
{"parentUuid":"u-1","type":"assistant","message":{"id":"m-1","type":"message","role":"assistant","model":"claude-opus-4-6","content":[{"type":"text","text":"hi"},{"type":"tool_use","id":"t-1","name":"Bash","input":{"cmd":"ls"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":100,"cache_read_input_tokens":50,"service_tier":"standard"}},"uuid":"a-1","timestamp":"2026-04-14T08:03:54.915Z","sessionId":"sess-1"}
{"parentUuid":"a-1","type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t-1","content":"ok"}]},"uuid":"u-2","timestamp":"2026-04-14T08:03:55.000Z","sessionId":"sess-1"}
not even json
`

func TestParseReader_HappyPath(t *testing.T) {
	var seen []RecordType
	var assistants []*AssistantMessage
	var toolUses int

	_, err := parseReader(strings.NewReader(sampleJSONL), func(r *Record) error {
		seen = append(seen, r.Type)
		if r.Type == TypeAssistant {
			am, err := DecodeAssistant(r)
			if err != nil {
				return err
			}
			assistants = append(assistants, am)
			for _, c := range am.Content {
				if c.Type == "tool_use" {
					toolUses++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parseReader error: %v", err)
	}

	wantTypes := []RecordType{TypePermissionMode, TypeUser, TypeAssistant, TypeUser}
	if len(seen) != len(wantTypes) {
		t.Fatalf("seen len = %d, want %d (%v)", len(seen), len(wantTypes), seen)
	}
	for i, w := range wantTypes {
		if seen[i] != w {
			t.Errorf("seen[%d] = %s, want %s", i, seen[i], w)
		}
	}
	if len(assistants) != 1 {
		t.Fatalf("expected 1 assistant, got %d", len(assistants))
	}
	a := assistants[0]
	if a.Model != "claude-opus-4-6" {
		t.Errorf("model = %q", a.Model)
	}
	if a.Usage.InputTokens != 10 || a.Usage.OutputTokens != 20 ||
		a.Usage.CacheCreationInputTokens != 100 || a.Usage.CacheReadInputTokens != 50 {
		t.Errorf("usage mismatch: %+v", a.Usage)
	}
	if toolUses != 1 {
		t.Errorf("tool_use count = %d, want 1", toolUses)
	}
}

func TestUserContentBlocks(t *testing.T) {
	stringContent := &UserMessage{Content: []byte(`"hello"`)}
	blocks, err := UserContentBlocks(stringContent)
	if err != nil || blocks != nil {
		t.Errorf("plain string should yield nil blocks (got %v, err %v)", blocks, err)
	}

	arrayContent := &UserMessage{Content: []byte(`[{"type":"tool_result","tool_use_id":"x","content":"ok"}]`)}
	blocks, err = UserContentBlocks(arrayContent)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Type != "tool_result" || blocks[0].ToolUseID != "x" {
		t.Errorf("blocks: %+v", blocks)
	}
}
