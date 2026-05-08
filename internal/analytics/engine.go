package analytics

import "context"

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
	// TODO: implement - use store to find session files
	return nil
}

func (e *Engine) getAllSessionFiles() []string {
	// TODO: implement - walk projects directory and find all JSONL files
	return nil
}

func (e *Engine) parseSessionFileForSkills(path string, skillCounts, pluginCounts map[string]int) error {
	// TODO: implement - parse JSONL and extract Skill tool params
	return nil
}
