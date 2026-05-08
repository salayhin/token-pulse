package analytics

type SkillUsage struct {
	Name       string  `json:"name"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
}

type SkillsBreakdownResult struct {
	Skills          []SkillUsage `json:"skills"`
	Plugins         []SkillUsage `json:"plugins"`
	TotalSkillCalls int          `json:"total_skill_calls"`
}
