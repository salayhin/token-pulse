package analytics

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) SkillsBreakdown(ctx context.Context, sessionID string) (*SkillsBreakdownResult, error) {
	// Parse JSONL files and extract skill usage
	skillCounts := make(map[string]int)
	pluginCounts := make(map[string]int)

	// If sessionID provided, analyze single session; otherwise analyze all sessions
	var filePaths []string
	if sessionID != "" {
		// Parse single session file - need to find it
		filePaths = e.findSessionFiles(sessionID)
	} else {
		// Parse all session files
		filePaths = e.getAllSessionFiles()
	}

	for _, path := range filePaths {
		if err := e.parseSessionFileForSkills(path, skillCounts, pluginCounts); err != nil {
			// Log but don't fail - some files may be unreadable
			continue
		}
	}

	skillPct := calculatePercentages(skillCounts)
	pluginPct := calculatePercentages(pluginCounts)

	return &SkillsBreakdownResult{
		Skills:          countsToSortedUsages(skillCounts, skillPct.Percentages),
		Plugins:         countsToSortedUsages(pluginCounts, pluginPct.Percentages),
		TotalSkillCalls: skillPct.Total,
	}, nil
}

func (e *Engine) findSessionFiles(sessionID string) []string {
	var files []string
	home, err := os.UserHomeDir()
	if err != nil {
		return files
	}
	claudeDir := filepath.Join(home, ".claude", "projects")

	filepath.Walk(claudeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			// Simple match: check if session ID is in filename
			if strings.Contains(path, sessionID) {
				files = append(files, path)
			}
		}
		return nil
	})

	return files
}

func (e *Engine) getAllSessionFiles() []string {
	var files []string
	home, err := os.UserHomeDir()
	if err != nil {
		return files
	}
	claudeDir := filepath.Join(home, ".claude", "projects")

	filepath.Walk(claudeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			files = append(files, path)
		}
		return nil
	})

	return files
}

func (e *Engine) parseSessionFileForSkills(path string, skillCounts, pluginCounts map[string]int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record map[string]interface{}
		if err := json.Unmarshal(line, &record); err != nil {
			continue // Skip malformed lines
		}

		// Only process assistant messages
		recordType, ok := record["type"].(string)
		if !ok || recordType != "assistant" {
			continue
		}

		// Extract message content
		msg, ok := record["message"].(map[string]interface{})
		if !ok {
			continue
		}

		// Extract content array from message
		content, ok := msg["content"].([]interface{})
		if !ok {
			continue
		}

		// Look for tool_use blocks
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, ok := blockMap["type"].(string)
			if !ok || blockType != "tool_use" {
				continue
			}

			// Extract skill from this tool use
			if skill, ok := extractSkillFromToolUse(blockMap); ok {
				skillCounts[skill]++

				// Also count plugin if it has one
				if plugin, hasPlugin := parsePluginFromSkill(skill); !hasPlugin {
					pluginCounts[plugin]++
				}
			}
		}
	}

	return scanner.Err()
}
