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
