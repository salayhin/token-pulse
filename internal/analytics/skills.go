package analytics

// extractSkillFromToolUse returns the skill name if this is a Skill tool use, or ("", false)
func extractSkillFromToolUse(toolUse map[string]interface{}) (string, bool) {
	// TODO: implement
	return "", false
}

// parsePluginFromSkill extracts the plugin namespace from a skill name
// Returns ("", true) if no plugin (e.g. "/init" or standalone)
func parsePluginFromSkill(skill string) (string, bool) {
	// TODO: implement
	return "", true
}
