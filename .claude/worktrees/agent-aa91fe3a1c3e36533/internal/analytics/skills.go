package analytics

// extractSkillFromToolUse returns the skill name if this is a Skill tool use, or ("", false)
func extractSkillFromToolUse(toolUse map[string]interface{}) (string, bool) {
	// Only process Skill tool
	name, ok := toolUse["name"].(string)
	if !ok || name != "Skill" {
		return "", false
	}

	// Extract input object
	input, ok := toolUse["input"].(map[string]interface{})
	if !ok {
		return "", false
	}

	// Extract skill parameter
	skill, ok := input["skill"].(string)
	if !ok || skill == "" {
		return "", false
	}

	return skill, true
}

// parsePluginFromSkill extracts the plugin namespace from a skill name
// Returns ("", true) if no plugin (e.g. "/init" or standalone)
func parsePluginFromSkill(skill string) (string, bool) {
	// Skills with ":" have a plugin namespace (e.g., "superpowers:brainstorming")
	for i, ch := range skill {
		if ch == ':' {
			plugin := skill[:i]
			if plugin != "" {
				return plugin, false
			}
		}
	}
	// No plugin for "/" prefixed skills or standalone names
	return "", true
}

func calculatePercentages(counts map[string]int) percentageResult {
	var total int
	for _, count := range counts {
		total += count
	}

	pcts := make(map[string]float64)
	if total > 0 {
		for name, count := range counts {
			pcts[name] = float64(count) / float64(total) * 100
		}
	}

	return percentageResult{Total: total, Percentages: pcts}
}

func countsToSortedUsages(counts map[string]int, percentages map[string]float64) []SkillUsage {
	usages := make([]SkillUsage, 0, len(counts))
	for name, count := range counts {
		usages = append(usages, SkillUsage{
			Name:       name,
			Count:      count,
			Percentage: percentages[name],
		})
	}
	// Sort by count descending
	for i := 0; i < len(usages); i++ {
		for j := i + 1; j < len(usages); j++ {
			if usages[j].Count > usages[i].Count {
				usages[i], usages[j] = usages[j], usages[i]
			}
		}
	}
	return usages
}
