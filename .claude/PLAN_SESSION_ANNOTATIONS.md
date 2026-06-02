# Plan: Session Annotation Feature

**Status:** Planned
**Estimated effort:** 4-5 hours
**Priority:** Low (nice-to-have UX enhancement)

## Goal

Let users add metadata to their Claude Code sessions for better organization and analysis. Currently, TokenPulse shows sessions as raw entries (timestamp, project slug, token count, cost). Users can't easily answer questions like:

- "How much did I spend on **client work** vs **personal projects** this month?"
- "Which sessions were **debugging** vs **feature development**?"
- "Show me all sessions tagged **#refactor**"

## Implementation

### 1. Database schema (`internal/store/`)

Add a new table linked to existing `sessions`:

```sql
CREATE TABLE session_annotations (
    session_id TEXT PRIMARY KEY,
    label TEXT,           -- e.g. "Client: Acme Corp"
    work_type TEXT,       -- e.g. "feature", "bugfix", "research"
    tags TEXT,            -- JSON array: ["#refactor", "#urgent"]
    notes TEXT,           -- free-form markdown
    updated_at INTEGER,
    FOREIGN KEY(session_id) REFERENCES sessions(id)
);
CREATE INDEX idx_annotations_work_type ON session_annotations(work_type);
```

Add a migration step in the store's schema setup. All writes must go through `writeMu` per the project's single-writer discipline invariant.

### 2. API endpoints (`internal/server/handlers/`)

- `PUT /api/v1/sessions/:id/annotation` — upsert annotation
- `DELETE /api/v1/sessions/:id/annotation` — remove
- `GET /api/v1/annotations/tags` — list all tags (for autocomplete)
- Update existing `/api/v1/sessions` to JOIN annotations and accept `?tag=`, `?work_type=`, `?label=` filters

All routes stay behind the 15s `middleware.Timeout` (no streaming).

### 3. UI changes (`web/`)

- Add **edit pencil** icon next to each session row
- Modal with: label input, work_type dropdown, tag chips input, notes textarea
- New **sidebar filters** for tags/work_type
- Tag chips render inline on session list

No new build step required — pure vanilla JS/HTML/CSS in keeping with project conventions.

### 4. CLI support (`cmd/tokenpulse/`)

```bash
tokenpulse sessions annotate <session-id> --label "Client X" --tag refactor
tokenpulse sessions list --tag urgent
tokenpulse stats --filter work_type=bugfix
```

Add as Cobra subcommands under the existing `sessions` group.

### 5. Analytics integration (`internal/analytics/`)

- Group cost/token rollups by `work_type` and tags
- Add aggregation queries that respect annotation filters
- Expose new endpoints (e.g. `/api/v1/stats?group_by=work_type`)

## Files to touch

| Layer | Files |
|-------|-------|
| Store | `internal/store/schema.go`, new `internal/store/annotations.go` |
| Server | `internal/server/handlers/annotations.go` (new), `internal/server/handlers/sessions.go` (modify), `internal/server/router.go` |
| Web | `web/index.html`, `web/static/app.js`, `web/static/styles.css` |
| CLI | `cmd/tokenpulse/sessions.go` (modify), new `cmd/tokenpulse/annotate.go` |
| Analytics | `internal/analytics/rollups.go` (modify) |
| Tests | One test file per layer |

## Rollout strategy

Suggested split into two PRs:

1. **PR 1: Foundation** — schema migration + API endpoints + CLI subcommand. Backend-only, fully tested.
2. **PR 2: UI & Analytics** — annotation modal, sidebar filters, grouped rollups.

This lets the backend ship and stabilize before UI work begins, and keeps each PR reviewable in under 30 minutes.

## Invariants to preserve

- All writes through `writeMu` (single-writer discipline)
- Server stays bound to `127.0.0.1`
- No CGo dependencies
- UTC-by-default for any timestamps in annotations
- Migration must be idempotent and forward-only

## Open questions

- Should tags be free-form or constrained to a user-defined vocabulary?
- Do we want bulk-annotation (e.g. "annotate all sessions in project X")?
- Should notes support markdown rendering in the UI, or plain text only?
- Auto-suggest work_type/tags based on session content (tool usage patterns)?
