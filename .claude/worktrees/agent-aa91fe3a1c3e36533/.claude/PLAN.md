# claude-token-lens — Project Plan & Roadmap (v3)

> v3 strips out team/multi-user scope — this is a **personal-stats-only** tool. v2 incorporated verification fixes (data path, Phase 0 schema discovery, SQLite-from-start, accurate cache math, vendored Chart.js, quantitative success metrics). See `PLAN_REVIEW.md` for the full audit trail.

## Progress

| Phase | Scope | Status | Notes |
|-------|-------|--------|-------|
| 0 | Schema discovery | ✅ Done (2026-05-08) | 12 record types catalogued, types.go locked against real JSONLs |
| 1 | Foundation, storage, dashboard parity | ✅ Done (2026-05-08) | Indexer 581ms / 12k msgs · binary 11MB · 4/4 tests green |
| 2 | Session explorer & search | ✅ Done (2026-05-08) | FTS5 1–2ms · session detail 4ms · live SSE 4s e2e · 9 new endpoints |
| 3 | Polish & personal power-user features | ✅ Done (2026-05-08) | Export, theme toggle, deep links, CI matrix, GoReleaser, Homebrew tap |

**🎉 All 4 phases complete.**

**Phase 2 verification snapshot:**
- New endpoints: `/projects`, `/projects/:slug/stats`, `/sessions`, `/sessions/:id`, `/sessions/search`, `/tools/:name`, `/stats/trends`, `/stats/projections`, `/events` (SSE)
- New CLI: `sessions {list,show,search}`, `tools show <name>`
- FTS5 search across 12,329 messages: 1–2ms first page (target <100ms)
- Session detail (491 messages, 167KB body): 4ms (target <500ms)
- File watcher → reindex → SSE event: ~4s end-to-end on file touch
- SPA: 4-tab UI (Overview, Projects, Sessions, Search), live indicator, click-through navigation
- Cost alerts: configurable daily budget with macOS notification (off by default)

**Phase 3 verification snapshot:**
- New endpoints: `/prompts`, `/models`, `/export?scope=daily|sessions|tools|projects&format=csv|json`
- New CLI: `export --scope ... --format ... -o file.csv`
- Theme toggle (◐) with `localStorage` persistence + system-preference fallback; full CSS-variable redesign
- Deep links via URL hash: `#sessions/<id>`, `#project/<slug>`, `#search/<query>` — share-able and bookmark-able
- CI: GitHub Actions matrix (macOS + Linux + Windows) running vet + race tests + golangci-lint v2
- Releases: GoReleaser config produces `darwin/{arm64,amd64}`, `linux/{amd64,arm64}`, `windows/amd64` archives + checksums
- Distribution: Homebrew tap auto-published via GoReleaser; one-command install `brew install <owner>/tap/claude-token-lens`
- Lint: 0 issues (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `misspell`, `gofmt`, `goimports`)

## Vision

A Go-powered **local, single-user** dashboard and CLI tool for monitoring, exploring, and analyzing your own Claude Code usage. Extends [claude-lens](https://github.com/foyzulkarim/claude-lens) with richer analytics, session exploration, and full-text search. Differentiates from [ccusage](https://github.com/ryoppippi/ccusage) by shipping a single Go binary with an embedded UI, full session-message browser, FTS5 search, and SQLite-backed analytics — not just a CLI cost report.

**Out of scope:** multi-user, team dashboards, auth, data upload, sharing. This tool runs locally on your machine against your own `~/.claude/`. If team mode is ever wanted, it's a separate project.

---

## Differentiation vs Prior Art

| Tool | Language | UI | Storage | Session Browser | FTS |
|------|----------|----|---------|-----------------|-----|
| claude-lens | Node | Static HTML | None (reparse) | No | No |
| ccusage | TypeScript | CLI only | None | No | No |
| **claude-token-lens** | **Go** | **Embedded SPA + CLI** | **SQLite** | **Yes** | **Yes (FTS5)** |

The pitch: one `go install`, one binary, full local analytics, no Node, no CGo.

---

## Data Source (Verified)

Claude Code stores sessions as JSONL files inside per-project directories:

```
~/.claude/projects/<slugified-cwd>/<session-uuid>.jsonl
```

Example: `~/.claude/projects/-Users-mbp258-projects-data-lab-claude-token-lens/abc-123.jsonl`

Each line is a JSON record. Known record types (to be confirmed in Phase 0):

- `user` — user prompt
- `assistant` — model response (contains `usage` object with token counts)
- `tool_use` / `tool_result` — embedded inside message `content` arrays
- `summary` — auto-compaction marker

Common fields: `parentUuid`, `sessionId`, `type`, `timestamp`, `cwd`, `gitBranch`, `version`, `message.usage.{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens}`.

**There is no `~/.claude/sessions/` directory.** Earlier drafts referenced one — incorrect.

---

## Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Language | Go 1.22+ | Single binary, fast startup, `net/http` method-pattern routing |
| HTTP Router | Chi v5 | Idiomatic middleware chain; could fall back to stdlib mux |
| Frontend | Single-page app (vanilla JS), embedded via `embed.FS` | Matches claude-lens model; no build step |
| Charts | Chart.js, **vendored** into `web/static/js/` | Truly self-contained binary; no CDN dependency |
| CLI | Cobra | Standard Go CLI framework |
| Config | Viper (env > flag > file) | `.env` + YAML + flag support |
| Storage | SQLite via `modernc.org/sqlite` | CGo-free for clean cross-compile. ~2–3× slower than `mattn/go-sqlite3` but acceptable for local-tool scale (≤100k messages) |
| Search | SQLite FTS5 (compiled into modernc) | No external search dep |
| File Watch | fsnotify | Per-OS abstraction; recursive watch with FD limit awareness |
| Logging | `slog` (stdlib) | Text for local, JSON optional |
| Testing | `testing` + `testify` | Standard |

---

## Architecture

```
claude-token-lens/
├── cmd/claude-token-lens/main.go      # Cobra entrypoint
├── internal/
│   ├── parser/         # JSONL → typed records
│   │   ├── types.go    # Record schema (locked in Phase 0)
│   │   ├── session.go
│   │   └── schema_test.go  # Validates real fixtures
│   ├── analytics/
│   │   ├── cost.go     # Token cost calc (correct cache formula)
│   │   ├── cache.go    # Hit rate + net savings
│   │   ├── tools.go
│   │   ├── trends.go   # Time-series + trailing-window projection
│   │   └── sessions.go
│   ├── store/
│   │   ├── store.go        # Storage interface
│   │   ├── sqlite.go       # SQLite implementation
│   │   ├── migrations.go
│   │   └── fts.go          # FTS5 index management
│   ├── server/
│   │   ├── server.go
│   │   ├── routes.go
│   │   ├── handlers/{dashboard,sessions,projects,search,export}.go
│   │   └── middleware/{cors,ratelimit,reqlog}.go
│   ├── watcher/watcher.go  # fsnotify with FD-limit fallback to polling
│   └── config/config.go    # Viper bindings + path resolution
├── web/
│   ├── static/
│   │   ├── css/app.css
│   │   └── js/{app.js, chart.umd.min.js}   # Chart.js vendored
│   ├── index.html          # Single SPA shell
│   └── embed.go            # //go:embed static index.html
├── testdata/
│   └── fixtures/           # Anonymized JSONL samples (regex-redacted)
├── configs/
│   ├── .env.example
│   └── config.yaml.example
├── .github/workflows/ci.yml
├── Makefile
├── go.mod / go.sum
├── CLAUDE.md
└── README.md
```

**Frontend approach:** single SPA (`index.html` + `app.js`) hitting JSON REST endpoints. No server-rendered templates. Matches claude-lens; minimizes complexity.

---

## Feature Roadmap

### Phase 0 — Schema Discovery ✅ Done

| # | Task |
|---|------|
| 0.1 | Dump 5+ real session JSONLs across different projects |
| 0.2 | Catalog every record `type` and field encountered |
| 0.3 | Lock `internal/parser/types.go` against real fixtures |
| 0.4 | Document schema version detection strategy (Claude Code may evolve format) |
| 0.5 | Anonymize 1–2 fixtures for `testdata/` (regex-redact API keys, user paths, emails) |

**Exit:** types.go compiles, parses real fixtures end-to-end, schema doc lives in `docs/schema.md`.

---

### Phase 1 — Foundation, Storage, Dashboard Parity ✅ Done

Goal: single binary that ingests JSONL into SQLite, serves the dashboard, and prints terminal stats.

| # | Feature | Priority | Notes |
|---|---------|----------|-------|
| 1.1 | `embed.FS` + Cobra skeleton | P0 | Set up first — structural |
| 1.2 | Config system (Viper, env > flag > YAML) | P0 | Set up first — structural |
| 1.3 | JSONL parser (uses Phase 0 types) | P0 | |
| 1.4 | SQLite store + migrations | P0 | sessions, messages, tool_calls, daily_rollup |
| 1.5 | Initial-load indexer (parse all → SQLite) | P0 | |
| 1.6 | Cost calculator with **correct cache formula** | P0 | See math below |
| 1.7 | Cache analytics (hit rate, net savings) | P0 | |
| 1.8 | Dashboard API: `/api/v1/stats`, `/stats/daily`, `/cache`, `/tools` | P0 | |
| 1.9 | Vendored Chart.js + SPA dashboard | P0 | Today/all-time, daily table, tools chart |
| 1.10 | CLI mode: `serve`, `stats`, `stats --today` | P0 | |
| 1.11 | Quantitative perf check | P0 | See success metrics |

**Cache savings math (locked):**
```
gross_saved   = cache_read_tokens   × (input_rate − cache_read_rate)
extra_paid    = cache_create_tokens × (cache_create_rate − input_rate)
net_saved_usd = gross_saved − extra_paid
```

**Success metric:** dashboard renders in <2s for 100 sessions / ~10k messages on M-series Mac. Indexer processes 10k messages in <5s.

---

### Phase 2 — Session Explorer & Search ✅ Done

| # | Feature | Priority |
|---|---------|----------|
| 2.1 | Per-project breakdown (cost, sessions, tool usage) | P0 |
| 2.2 | Time-series trends (daily/weekly/monthly + 7d MA) | P0 |
| 2.3 | Cost projections (trailing-7-day average × days-in-month, documented) | P1 |
| 2.4 | Session list (paginated: `?limit=50&cursor=`) | P0 |
| 2.5 | Session detail view (message thread, per-turn tokens) | P0 |
| 2.6 | FTS5 full-text search across messages | P1 |
| 2.7 | Search filters: project, date range, type | P1 |
| 2.8 | Tool call deep dive (per-tool stats) | P1 |
| 2.9 | fsnotify watcher → SSE live updates | P1 |
| 2.10 | Cost alerts (terminal notifications) | P2 |

**Success metric:** FTS search returns first page in <100ms over 10k messages. Session detail loads in <500ms.

---

### Phase 3 — Polish & Personal Power-User Features ✅ Done

| # | Feature | Priority |
|---|---------|----------|
| 3.1 | CSV / JSON export | P1 |
| 3.2 | Prompt analytics (avg length, prompt/response ratio) | P1 |
| 3.3 | Multi-model comparison (if multiple models used) | P2 |
| 3.4 | Saved views (filtered dashboard URLs) | P2 |
| 3.5 | Dark/light theme | P1 |
| 3.6 | Local-only desktop notifications on budget threshold (no webhooks) | P2 |
| 3.7 | `/health` endpoint (for personal monitoring scripts) | P2 |
| 3.8 | Structured `slog` output | P1 |
| 3.9 | Graceful shutdown | P1 |
| 3.10 | GitHub Actions CI + golangci-lint | P0 |
| 3.11 | Cross-platform release builds (macOS arm64/amd64, Linux, Windows) | P1 |
| 3.12 | Homebrew tap for `brew install claude-token-lens` | P2 |

---

## API Design

```
GET  /api/v1/stats                      # Today + all-time aggregates
GET  /api/v1/stats/daily?from=&to=      # Daily breakdown
GET  /api/v1/stats/trends?window=7d     # Time-series
GET  /api/v1/stats/projections          # Trailing-window projection

GET  /api/v1/cache                       # Cache metrics (with correct net savings)
GET  /api/v1/tools                       # Tool call counts
GET  /api/v1/tools/:name                 # Per-tool deep dive

GET  /api/v1/projects
GET  /api/v1/projects/:slug/stats

GET  /api/v1/sessions?project=&limit=50&cursor=
GET  /api/v1/sessions/:id
GET  /api/v1/sessions/search?q=&project=&from=&to=

GET  /api/v1/export?format=csv|json&scope=daily|sessions|tools

GET  /api/v1/health
GET  /api/v1/events                      # SSE — under /api/v1/ for consistency
```

**Cross-cutting:** server binds to `127.0.0.1` only (loopback). Pagination is cursor-based. Rate limiting omitted — single-user local tool.

---

## CLI Commands

Duration syntax: `--last` accepts `7d`, `24h`, `30m` via custom parser (`d` is not valid in `time.ParseDuration`).

```bash
claude-token-lens serve [--port 3456] [--claude-dir ~/.claude] [--tz UTC]

claude-token-lens stats [--today] [--last 7d] [--project <slug>] [--tz local]
claude-token-lens cost  [--daily] [--project <slug>] [--format csv|json]

claude-token-lens sessions list [--project <slug>] [--last 7d]
claude-token-lens sessions show <session-id>
claude-token-lens sessions search "<query>" [--project <slug>]

claude-token-lens tools [--top 10]

claude-token-lens export --format csv|json [--scope daily|sessions|tools] --output report.csv

claude-token-lens config init
claude-token-lens config set pricing.input 5.0
claude-token-lens config show

claude-token-lens index            # Incremental
claude-token-lens index --rebuild  # Drop & reindex
```

**Timezone:** all date bucketing defaults to **UTC**. Override with `--tz local` or `tz: "Asia/Tokyo"` in config. Documented explicitly because daily totals shift at boundaries.

---

## Configuration

**Lookup order (Viper):** flags → env (`CTL_*`) → `./config.yaml` → `~/.config/claude-token-lens/config.yaml` → defaults.

```yaml
claude_dir: "~/.claude"
timezone: "UTC"        # or "Asia/Tokyo", "America/Los_Angeles"

server:
  port: 3456
  host: "127.0.0.1"

pricing:
  preset: "anthropic-api"   # or "bedrock-<region>" or "custom"
  input: 3.0
  output: 15.0
  cache_read: 0.30
  cache_create: 3.75

storage:
  path: "~/.config/claude-token-lens/data.db"

watcher:
  enabled: true
  debounce_ms: 250

alerts:
  daily_budget: 50.0        # local desktop notification only
```

**Bedrock region:** if using Bedrock, set `preset: "bedrock-ap-northeast-1"` (Tokyo) or whichever region matches your account. The previous `ap-southeast-2` example was Sydney.

---

## Data Flow

```
~/.claude/projects/<slug>/<uuid>.jsonl
        │
        ▼  (initial scan + fsnotify)
   ┌───────────┐
   │  Parser   │  internal/parser
   └─────┬─────┘
         │  records
         ▼
   ┌───────────┐
   │  Store    │  internal/store (SQLite + FTS5)
   │  - sessions, messages, tool_calls
   │  - daily_rollup (materialized)
   │  - messages_fts
   └─────┬─────┘
         │
   ┌─────┴───────────────┐
   ▼                     ▼
┌─────────┐         ┌─────────┐
│Analytics│         │  HTTP   │
└────┬────┘         │ Server  │
     │              └────┬────┘
     │                   │
     ▼                   ├──► JSON API ──► SPA Dashboard
  CLI Output             └──► SSE (/events) ──► live UI updates
```

---

## Development

```bash
go mod init github.com/<org>/claude-token-lens

go get github.com/spf13/cobra github.com/spf13/viper \
       github.com/go-chi/chi/v5 modernc.org/sqlite \
       github.com/fsnotify/fsnotify github.com/stretchr/testify

go install github.com/air-verse/air@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

**Makefile:**

```makefile
.PHONY: build run test lint fixtures clean

build:
	go build -trimpath -ldflags="-s -w" -o bin/claude-token-lens ./cmd/claude-token-lens

run:
	go run ./cmd/claude-token-lens serve

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

fixtures:
	go run ./internal/parser/cmd/anonymize ~/.claude/projects > testdata/fixtures/sample.jsonl

clean:
	rm -rf bin/
```

**Cross-platform notes:**
- macOS: fsnotify uses kqueue (FD per dir). With >256 project dirs, fall back to 30s polling.
- Linux: inotify (default limit ~8192 watches; sufficient).
- Windows: `ReadDirectoryChangesW` (handled by fsnotify).

---

## Success Metrics (Quantitative)

| Phase | Metric |
|-------|--------|
| 0 | `parser` package passes against 3+ real anonymized fixtures |
| 1 | Indexer ingests 10k messages in <5s; dashboard renders in <2s for 100 sessions; binary <30MB |
| 2 | FTS search <100ms over 10k messages; session detail loads <500ms |
| 3 | Cold start <500ms; CI green on macOS + Linux + Windows; `brew install` produces working binary |

---

## Key Decisions

1. **Personal-use only** — no auth, no multi-user, no team mode. Server binds to `127.0.0.1` only. If team mode is ever wanted, it's a separate project (privacy/redaction is a hard design problem).
2. **SQLite from day one** — original plan deferred this to Phase 2; the rationale ("parse once, query fast") is a Phase-1 concern.
3. **Vendored Chart.js, not CDN** — "single binary" only means something if it actually runs offline.
4. **SPA, not server-rendered** — one HTML file + JSON API. Matches claude-lens; halves the work.
5. **`modernc.org/sqlite` chosen for portability, not speed** — clean cross-compile beats raw QPS for a local tool.
6. **UTC by default** — daily totals must be deterministic across machines.

---

## Open Questions

- Final repo URL / module path — pending.
- Schema versioning: detect Claude Code version from `version` field? Pin compatible range?
- Should `serve` auto-open browser on launch (`open http://localhost:3456`)?
