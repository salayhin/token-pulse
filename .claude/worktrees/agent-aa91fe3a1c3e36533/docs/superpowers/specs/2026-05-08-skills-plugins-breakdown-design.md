---
name: Skills & Plugins Usage Breakdown
description: Display skill and plugin usage statistics on Overview and Session detail pages
type: design
date: 2026-05-08
---

# Skills & Plugins Usage Breakdown

## Overview

Add Skills and Plugins usage breakdown sections to the Overview and Session detail pages, showing top 10 items by frequency with expandable "Show all" option.

## Requirements

- **Display**: Text list format with skill/plugin names and percentage of total skill invocations
- **Placement**: Overview page and Session detail page
- **Limit**: Top 10 items shown by default, expandable to show all
- **Percentage basis**: % of total Skill tool invocations
- **Data source**: Parse raw JSONL files to extract Skill tool parameters (accurate, not inferred)

## Architecture

### Backend

#### Data Extraction

1. **Skill Parameter Parsing**
   - Parse JSONL files (same pattern as indexer)
   - Extract tool_use records where `tool.name == "Skill"`
   - Get `skill` parameter from `tool.input` object
   - Example: when `Skill` tool is invoked with `skill: "superpowers:brainstorming"`, capture that skill name

2. **Categorization**
   - **Skills**: Full skill invocation name (e.g., `superpowers:brainstorming`, `/init`)
   - **Plugins**: Namespace prefix extracted from skill name
     - `superpowers:brainstorming` → plugin: `superpowers`
     - `/init` → plugin: (no plugin, skip if no colon/namespace)
     - Extraction: take text before the first `:` if present, or handle `/`-prefixed items as standalone

3. **Counting & Aggregation**
   - Count each unique skill invocation
   - Sum counts for each plugin (multiple skills can share a plugin)
   - Calculate percentages: `(count / total_skill_calls) * 100`
   - Total = sum of all individual Skill tool invocations

#### New Analytics Method

Add `SkillsBreakdown()` to `internal/analytics/engine.go`:

```go
type SkillsBreakdownResult struct {
    Skills           []SkillUsage `json:"skills"`
    Plugins          []SkillUsage `json:"plugins"`
    TotalSkillCalls  int          `json:"total_skill_calls"`
}

type SkillUsage struct {
    Name       string  `json:"name"`
    Count      int     `json:"count"`
    Percentage float64 `json:"percentage"`
}

// SkillsBreakdown returns skill/plugin usage for a time period or session
func (e *Engine) SkillsBreakdown(ctx context.Context, sessionID string, from, to time.Time) (*SkillsBreakdownResult, error)
```

- If `sessionID` is provided: analyze only that session's JSONL file
- If `from`/`to` are provided: analyze all sessions in that date range
- If neither: analyze all sessions (all-time)

#### HTTP Endpoints

Add to `internal/server/handlers/handlers.go`:

- `GET /api/v1/skills[?from=YYYY-MM-DD&to=YYYY-MM-DD]` — all-time or date-range breakdown
- `GET /api/v1/sessions/<sessionId>/skills` — breakdown for specific session

Both return `SkillsBreakdownResult` JSON.

### Frontend

#### HTML Structure

Add two new sections to `web/index.html` in the Overview view (after the Tools chart):

```html
<div class="chart-box">
  <h2>Skills</h2>
  <div id="skills-list" class="skills-plugins-list"></div>
  <button id="skills-show-all" class="toggle-btn hidden">Show all</button>
  <div id="skills-all" class="skills-plugins-all hidden"></div>
</div>

<div class="chart-box">
  <h2>Plugins</h2>
  <div id="plugins-list" class="skills-plugins-list"></div>
  <button id="plugins-show-all" class="toggle-btn hidden">Show all</button>
  <div id="plugins-all" class="skills-plugins-all hidden"></div>
</div>
```

Add same structure to Session detail view (in `web/index.html` section for sessions).

#### CSS

Add to `web/static/css/app.css`:

```css
.skills-plugins-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  font-size: 0.9rem;
}

.skills-plugins-item {
  display: flex;
  justify-content: space-between;
  padding: 0.25rem 0;
}

.skills-plugins-item .name {
  flex: 1;
  word-break: break-word;
}

.skills-plugins-item .percentage {
  margin-left: 1rem;
  text-align: right;
  min-width: 4rem;
  color: #5fcfa6;
}

.toggle-btn {
  margin-top: 0.5rem;
  padding: 0.25rem 0.5rem;
  font-size: 0.85rem;
}

.skills-plugins-all {
  margin-top: 0.5rem;
  max-height: 300px;
  overflow-y: auto;
}

.skills-plugins-all.hidden {
  display: none;
}

.toggle-btn.hidden {
  display: none;
}
```

#### JavaScript Logic

Add to `web/static/js/app.js`:

```javascript
function renderSkillsList(data, elementId, allElementId, toggleId) {
  const list = document.getElementById(elementId);
  const allList = document.getElementById(allElementId);
  const toggle = document.getElementById(toggleId);
  
  // Top 10 items
  const top10 = data.slice(0, 10);
  const hasMore = data.length > 10;
  
  list.innerHTML = top10.map(item => `
    <div class="skills-plugins-item">
      <span class="name">${escapeHtml(item.name)}</span>
      <span class="percentage">${item.percentage.toFixed(1)}%</span>
    </div>
  `).join('');
  
  // All items (hidden by default)
  if (hasMore) {
    allList.innerHTML = data.map(item => `
      <div class="skills-plugins-item">
        <span class="name">${escapeHtml(item.name)}</span>
        <span class="percentage">${item.percentage.toFixed(1)}%</span>
      </div>
    `).join('');
    toggle.classList.remove('hidden');
    toggle.addEventListener('click', () => {
      allList.classList.toggle('hidden');
      toggle.textContent = allList.classList.contains('hidden') ? 'Show all' : 'Hide';
    });
  }
}

async function loadOverviewSkills() {
  const data = await getJSON('/api/v1/skills');
  renderSkillsList(data.skills, 'skills-list', 'skills-all', 'skills-show-all');
  renderSkillsList(data.plugins, 'plugins-list', 'plugins-all', 'plugins-show-all');
}

async function loadSessionSkills(sessionId) {
  const data = await getJSON(`/api/v1/sessions/${encodeURIComponent(sessionId)}/skills`);
  renderSkillsList(data.skills, 'session-skills-list', 'session-skills-all', 'session-skills-show-all');
  renderSkillsList(data.plugins, 'session-plugins-list', 'session-plugins-all', 'session-plugins-show-all');
}
```

Update `loadOverview()` to call `loadOverviewSkills()`.

Update session detail loading to call `loadSessionSkills(sessionId)`.

## Data Flow

1. User visits Overview → `loadOverviewSkills()` calls `GET /api/v1/skills`
2. Backend `SkillsBreakdown()` parses all JSONL files
3. Extracts Skill tool parameters → counts → calculates percentages
4. Returns JSON with top 10 skills/plugins (all included in response)
5. Frontend renders top 10, hides remaining, shows "Show all" button if count > 10

Same flow for Session detail page with session-specific data.

## Error Handling

- If no Skill tool invocations found: return `total_skill_calls: 0`, empty arrays
- If JSONL parsing fails: log error, return empty arrays (don't crash)
- Network errors on frontend: display "—" like other metric failures

## Testing

- Unit: Verify skill/plugin parsing and percentage calculations
- Integration: Verify endpoints return correct data for all-time and session-specific queries
- UI: Verify rendering, expand/collapse toggle, escaping of skill names

## Implementation Order

1. Add `SkillsBreakdown()` method in analytics
2. Add HTTP endpoints
3. Add HTML/CSS to Overview and Session pages
4. Add JavaScript rendering and API calls
5. Test end-to-end
