package parser

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const previewMaxRunes = 200

// AssistantText concatenates all text + thinking blocks of an assistant message.
func AssistantText(m *AssistantMessage) string {
	var sb strings.Builder
	for _, c := range m.Content {
		switch c.Type {
		case "text":
			if c.Text != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(c.Text)
			}
		case "thinking":
			if c.Thinking != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(c.Thinking)
			}
		}
	}
	return sb.String()
}

// UserText returns the user prompt as a string. For tool_result content arrays,
// returns "" so they don't pollute search.
func UserText(m *UserMessage) string {
	if len(m.Content) == 0 {
		return ""
	}
	if m.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			return s
		}
	}
	if m.Content[0] == '[' {
		var blocks []ContentBlock
		if err := json.Unmarshal(m.Content, &blocks); err == nil {
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" && b.Text != "" {
					if sb.Len() > 0 {
						sb.WriteByte('\n')
					}
					sb.WriteString(b.Text)
				}
			}
			return sb.String()
		}
	}
	return ""
}

// Preview returns the first N runes of s, single-line, with whitespace collapsed.
func Preview(s string) string {
	if s == "" {
		return ""
	}
	// Collapse whitespace to single spaces.
	var sb strings.Builder
	sb.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		sb.WriteRune(r)
	}
	out := strings.TrimSpace(sb.String())
	if utf8.RuneCountInString(out) <= previewMaxRunes {
		return out
	}
	// Truncate at rune boundary.
	count := 0
	for i := range out {
		if count == previewMaxRunes {
			return out[:i] + "…"
		}
		count++
	}
	return out
}
