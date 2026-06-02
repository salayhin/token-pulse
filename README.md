# tokenpulse

A Go-powered local dashboard and CLI for monitoring Claude Code token usage, costs, and cache savings.
Single binary, embedded UI, no external dependencies.

> Personal use only. Server binds to `127.0.0.1` and reads your local
> `~/.claude/projects/` JSONL files. No multi-user, no auth, no upload.

## Quick start

```bash
make build
./bin/tokenpulse index           # one-time ingest into SQLite
./bin/tokenpulse stats           # all-time stats
./bin/tokenpulse stats --today   # today only
./bin/tokenpulse serve           # dashboard at http://127.0.0.1:3456
```

## Features

- **Real-time dashboard** — live token usage, cost, and cache stats via SSE
- **Session explorer** — browse all Claude Code sessions with per-turn token breakdowns
- **Model breakdown** — input/output tokens and cost per model in the Overview
- **Daily trends** — 30-day cost chart with 7-day moving average
- **Cache analytics** — hit rate, read/create volumes displayed in M tokens
- **Skills & plugins** — tracks which superpowers skills and plugins are invoked
- **Settings** — live pricing and timezone editing; no restart needed
- **Export** — CSV or JSON export for daily stats, sessions, and projects

## System Architecture

```
~/.claude/projects/<slug>/<session>.jsonl
        │
   parser  ──► indexer ──► store (SQLite + FTS5)
                              │
                              ├─► analytics ──► server (chi REST + SSE)
                              │                       │
                              │                       └─► embedded SPA (web/)
                              └─► analytics ──► CLI (cobra)

   watcher (fsnotify) ──► debounced reindex ──► EventBus.Publish("updated") ──► SSE clients + alerts.Check
```

### Layers

**Source Data** — Claude Code writes one JSONL file per session under `~/.claude/projects/<slug>/`. Each line is a typed record (user message, assistant message, tool call, usage stats, etc.).

**Ingest Layer**
- `parser` — Converts raw JSONL lines into typed `Record` structs. Resume-safe: `ParseFile` returns the byte offset after the last *complete* line so a partial trailing line is never silently dropped.
- `indexer` — Walks all project subdirectories. Two-mode strategy per file: *incremental* (size+mtime unchanged → skip; size grew → parse from `state.LastOffset`) or *full rebuild* (`--rebuild` flag). Cost per watcher tick is O(new bytes).
- `watcher` — fsnotify listener on the projects directory. Debounces write bursts (800 ms). New project subdirs are auto-watched as they appear.

**Storage Layer** — `modernc.org/sqlite` (CGo-free, cross-compiles on Linux/macOS/Windows). WAL mode, `busy_timeout=30000`, `MaxOpenConns=8`. A `writeMu` mutex serializes all writes. Inserts are batched at ≤500 rows per transaction. Schema: `projects`, `sessions`, `messages`, `tool_calls`, `messages_fts` (FTS5), `file_state` (resume offsets).

**Analytics Layer** — Read-only queries on SQLite. `cost.go` holds the locked cost and cache-savings formulas. The daily-budget alert check re-runs on every `"updated"` EventBus event and fires a macOS notification when the threshold is crossed.

**Serve Layer**
- `server` — chi router with a 15 s `middleware.Timeout` on bounded routes. `/export` and `/events` (SSE) are explicitly outside the timeout.
- Embedded SPA — `embed.FS`-served `index.html` + `static/`. Charts via vendored Chart.js. No build step required.
- `config` — Viper. Lookup order: flag → env (`TP_*`) → `./config.yaml` → `~/.config/tokenpulse/config.yaml` → defaults. `PricingFor(model)` uses longest-prefix matching, then `Pricing.Fallback`.

## Configuration

Copy `configs/config.yaml.example` to `./config.yaml` (or `~/.config/tokenpulse/config.yaml`) and edit.
Env vars (`TP_*`) also work, e.g. `TP_TIMEZONE=Asia/Tokyo TP_SERVER_PORT=4000`.

## Data paths

- Source: `~/.claude/projects/<slug>/<session-uuid>.jsonl`
- Database: `~/.config/tokenpulse/data.db`
- Config: `~/.config/tokenpulse/config.yaml`

Re-running `index` is incremental (skips files unchanged by size+mtime). Use `index --rebuild` to force a full re-index (e.g. after a pricing update).

## Cache savings math

Cache creation tokens are split by TTL tier because Anthropic charges them at different rates:

```
gross_saved   = cache_read_tokens × (input_rate − cache_read_rate)
extra_paid_5m = cache_create_5m   × (cache_create_5m_rate − input_rate)
extra_paid_1h = cache_create_1h   × (cache_create_1h_rate − input_rate)
net_saved     = gross_saved − extra_paid_5m − extra_paid_1h
```

Pre-migration rows have only the legacy `cache_create_tokens` column populated; `splitCacheCreate()` treats the unallocated portion as 5 min to preserve historical numbers. Run `index --rebuild` to backfill the split columns.

## Key invariants

- **127.0.0.1 only.** Personal-use tool; do not expose to a network interface or add auth/multi-user concepts.
- **UTC by default.** All date bucketing uses `cfg.Location()`; override via `timezone:` config key or `TP_TIMEZONE`.
- **Resume offsets matter.** Partial lines never advance the offset — keeps incremental indexing correct under concurrent appends from Claude Code.
- **No CGo.** `modernc.org/sqlite` for clean cross-compile (`CGO_ENABLED=0`). Do not introduce a CGo dependency.
- **Single writer discipline.** All store writes must acquire `writeMu`. Long batches must split into ≤500-row transactions.

## Development

```bash
make build              # CGO-disabled binary at bin/tokenpulse
make run                # go run ./cmd/tokenpulse serve
make index              # one-time ingest of ~/.claude into SQLite
make stats              # all-time terminal stats
make test               # go test ./... -race -count=1
make lint               # golangci-lint run (config in .golangci.yml)
make tidy               # go mod tidy
make snapshot           # goreleaser release --snapshot --clean
make clean              # rm -rf bin/ dist/
```

Single package test:

```bash
go test ./internal/parser -run TestParseFile -race -count=1
go test ./internal/indexer -race -count=1
```

## CI / Release

CI matrix (`.github/workflows/ci.yml`): Ubuntu + macOS + Windows — `go vet`, `go test -race`, build, and `golangci-lint`. Releases via GoReleaser (`.goreleaser.yaml`); Homebrew tap auto-published.
