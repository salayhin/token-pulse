package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ParseFile reads a session JSONL starting at fromOffset and yields each
// record via fn. Returns the byte offset *after* the last complete line
// successfully read. A partial trailing line (no '\n') does NOT advance the
// returned offset, so callers can safely persist it as a resume point —
// when the file is appended to and reparsed from this offset, the partial
// line will be re-read in full.
//
// Malformed JSON is skipped silently (the caller's offset still advances
// past those bytes; only complete lines count).
func ParseFile(path string, fromOffset int64, fn func(rec *Record) error) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return fromOffset, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if fromOffset > 0 {
		if _, err := f.Seek(fromOffset, io.SeekStart); err != nil {
			return fromOffset, fmt.Errorf("seek %s: %w", path, err)
		}
	}
	consumed, err := parseReader(f, fn)
	return fromOffset + consumed, err
}

// parseReader streams records from r and returns the number of bytes
// consumed up to (and including) the last complete line. A trailing partial
// line (EOF without a '\n') is intentionally NOT counted, so the caller's
// resume offset stays at the start of the partial line.
func parseReader(r io.Reader, fn func(rec *Record) error) (int64, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	var consumed int64
	for {
		line, err := br.ReadBytes('\n')
		complete := err == nil // ReadBytes returns nil err only when delim was found
		if complete && len(line) > 0 {
			consumed += int64(len(line))
			rec := &Record{}
			if jErr := json.Unmarshal(line, rec); jErr == nil && rec.Type != "" {
				if cbErr := fn(rec); cbErr != nil {
					return consumed, cbErr
				}
			}
		}
		if err == io.EOF {
			return consumed, nil
		}
		if err != nil {
			return consumed, err
		}
	}
}

// DecodeAssistant parses the embedded assistant message.
func DecodeAssistant(rec *Record) (*AssistantMessage, error) {
	if rec.Type != TypeAssistant || len(rec.Message) == 0 {
		return nil, fmt.Errorf("not an assistant record")
	}
	var m AssistantMessage
	if err := json.Unmarshal(rec.Message, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeUser parses the embedded user message.
func DecodeUser(rec *Record) (*UserMessage, error) {
	if rec.Type != TypeUser || len(rec.Message) == 0 {
		return nil, fmt.Errorf("not a user record")
	}
	var m UserMessage
	if err := json.Unmarshal(rec.Message, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// UserContentBlocks attempts to interpret the user message content as a block array.
// Returns nil if content is a plain string.
func UserContentBlocks(m *UserMessage) ([]ContentBlock, error) {
	if len(m.Content) == 0 {
		return nil, nil
	}
	if m.Content[0] == '[' {
		var cbs []ContentBlock
		if err := json.Unmarshal(m.Content, &cbs); err != nil {
			return nil, err
		}
		return cbs, nil
	}
	return nil, nil
}

// ListSessionFiles returns every *.jsonl file under claudeProjectsDir.
// claudeProjectsDir is typically ~/.claude/projects.
func ListSessionFiles(claudeProjectsDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(claudeProjectsDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(p) == ".jsonl" {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ProjectSlugFromPath takes ~/.claude/projects/<slug>/<uuid>.jsonl and returns <slug>.
func ProjectSlugFromPath(p string) string {
	return filepath.Base(filepath.Dir(p))
}
