# Plan Verification Notes (v1 → v2 → v3)

This document captures the audits that produced `PLAN.md`. Kept for traceability — not a living doc.

## v3 change (scope cut)

**Decision:** team/multi-user mode dropped entirely. Tool is now personal-stats-only.

**Removed:**
- Phase 3 (read-only team dashboard)
- JWT/bcrypt/auth middleware
- `/api/v1/auth/*` and `/api/v1/team/*` endpoints
- Webhook alerts (Slack/Discord) — replaced with local desktop notifications
- Docker image (no shared deployment target)
- Postgres adapter / multi-writer concerns
- Rate limiting (single-user, loopback-bound)
- `auth:` config block

**Renumbered:** old Phase 4 (polish) → new Phase 3.

**Added:** loopback-only bind (`127.0.0.1`), Homebrew tap as a Phase 3 task.

**Rationale:** privacy/redaction for multi-user upload is a hard design problem worth doing right or not at all. Personal scope keeps the tool sharp.

## Blockers fixed

| # | Issue (v1) | Fix (v2) |
|---|------------|----------|
| 1 | Read from `~/.claude/sessions/` — directory does not exist | Removed. Sessions live as JSONL inside `~/.claude/projects/<slug>/<uuid>.jsonl` |
| 2 | JSONL schema undocumented | Added **Phase 0 — Schema Discovery** with fixture validation |
| 3 | Phase 1 "feature parity" contradicted "Why SQLite" rationale | SQLite + indexer moved into Phase 1 |
| 4 | Cache savings formula was wrong (`reads × input_rate`) | Locked correct formula: `gross_saved − extra_paid_for_creation` |
| 5 | Phase 3 data upload had zero privacy story | Phase 3 descoped to **read-only** team dashboard. Per-user upload deferred behind a redaction-pipeline design doc |

## Significant issues fixed

| # | Issue | Fix |
|---|-------|-----|
| 6 | Prior art (`ccusage`) not acknowledged | Added differentiation table |
| 7 | `modernc.org/sqlite` called "fast" | Reframed: chosen for portability; ~2–3× slower than CGo, acceptable at local-tool scale |
| 8 | Frontend mixed templates + SPA | Locked: single SPA + JSON API |
| 9 | Phase 3 timeline (2 weeks) was unrealistic | Extended to Week 5–8 and descoped |
| 10 | SQLite single-writer for multi-user | Added storage-interface design; Postgres adapter as escape hatch |
| 11 | `--last 7d` — `d` not valid in `time.ParseDuration` | Custom duration parser noted in CLI section |
| 12 | `bedrock-ap-southeast-2` (Sydney) for Japan-based user | Default preset switched to `anthropic-api`; Tokyo example called out |
| 13 | Chart.js via CDN broke "single binary" claim | Vendored into `web/static/js/` |
| 14 | Timezone unspecified | UTC default, `--tz` flag, config field |

## Smaller fixes applied

- Pagination: cursor-based, query params documented
- Rate limiting: token bucket, 60 rpm default, all endpoints
- `jwt_secret` removed from YAML; env-only
- Config lookup order documented (flags → env → cwd → XDG → defaults)
- CI / lint / fixture anonymization added (Phase 4.11)
- fsnotify FD-limit fallback to polling noted
- Phase 1 ordering: embed + config moved to first
- Cost projection algorithm specified (trailing-7-day × days-in-month)
- Success metrics made quantitative
- `/events` SSE moved under `/api/v1/`
- Cross-platform notes added

## Unresolved (open questions in v3)

- Final repo URL / module path
- Claude Code schema-version detection strategy
- Whether `serve` should auto-open the browser

## What was NOT changed

- Cobra + Viper + Chi stack — sound choices
- Single-binary, embedded-frontend ethos — preserved
- Phase 4 polish list — mostly intact
- API surface — refined but structurally same
