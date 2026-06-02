# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

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

# Single test or package
go test ./internal/parser -run TestParseFile -race -count=1
go test ./internal/indexer -race -count=1
```

After `make build`:
- `./bin/tokenpulse index [--rebuild]` — incremental indexing; `--rebuild` forces full re-index
- `./bin/tokenpulse serve [--skip-index]` — dashboard at `http://127.0.0.1:3456`
- `./bin/tokenpulse stats [--today]`, `sessions {list,show,search}`, `tools show <name>`, `export --scope ... --format ...`

## Architecture

Single Go binary with embedded SPA. Data flow:

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

### Package layout

- `cmd/tokenpulse/main.go` — Cobra entrypoint; wires every subcommand. The `serve` command also constructs an `alertingBus` shim that wraps `handlers.EventBus` so each `"updated"` event re-runs the daily-budget alert check.
- `internal/parser/` — JSONL → typed `Record`s. `ParseFile` is **resume-safe**: returns the byte offset *after* the last complete line; a partial trailing line does NOT advance the offset, so re-parsing from that offset re-reads the partial line in full. `types.go` is locked against real fixtures.
- `internal/indexer/` — Walks `<claudeDir>/projects`, two-mode ingest per file: **incremental** (size+mtime unchanged → skip; size grew → parse from `state.LastOffset` only) or **full rebuild** (first-time, truncated, or `force=true`). Cost of a watcher tick is O(new bytes), not O(session size).
- `internal/store/` — SQLite via `modernc.org/sqlite` (CGo-free). WAL mode, `busy_timeout=30000`, `MaxOpenConns=8`. `writeMu` serializes writes at the application layer to keep idle conns free for readers under contention. Inserts are batched at `insertMessagesBatch=500` per tx so the writer slot releases frequently. Slow writes (>1s) are counted in `slowWrites` and surfaced via `/api/v1/health`. Schema: `projects`, `sessions`, `messages`, `tool_calls`, `messages_fts` (FTS5), `file_state` (resume offsets).
- `internal/analytics/` — Read-side. `cost.go` holds the **locked cost & cache-savings formulas** (do not modify without updating README and tests). The cache-creation column is split by ephemeral TTL because Anthropic charges them at different rates:
  ```
  gross_saved   = cache_read_tokens × (input_rate − cache_read_rate)
  extra_paid_5m = cache_create_5m   × (cache_create_5m_rate − input_rate)
  extra_paid_1h = cache_create_1h   × (cache_create_1h_rate − input_rate)
  net_saved     = gross_saved − extra_paid_5m − extra_paid_1h
  ```
  Pre-migration rows have only the legacy `cache_create_tokens` populated; `splitCacheCreate()` treats the unallocated portion as 5m to preserve historical cost numbers. Run `index --rebuild` to backfill the split columns and get accurate 1h pricing on old data.
- `internal/server/` — chi router. Bounded routes have a 15s `middleware.Timeout`; `/export` and `/events` (SSE) are explicitly outside the timeout because they are streaming.
- `internal/server/handlers/eventbus.go` — Tiny SSE fan-out. Subscriber channels are buffered (16); a slow subscriber's events are **dropped**, never blocked.
- `internal/watcher/` — fsnotify on `<claudeDir>/projects`. Debounces bursts (800ms) and uses a `pending` flag so concurrent file events do not stack reindex goroutines on SQLite's single writer. New project subdirs are auto-watched as they appear.
- `internal/alerts/` — Daily-budget threshold check; macOS `osascript` notification when `alerts.notify=true`.
- `internal/config/` — Viper. Lookup order: flag → env (`TP_*`) → `./config.yaml` → `~/.config/tokenpulse/config.yaml` → defaults. `PricingFor(model)` falls back to a longest-prefix match (e.g. `claude-sonnet-4-...` → `claude-sonnet-4` rates), then `Pricing.Fallback`.
- `web/` — `embed.FS`-served SPA (`index.html` + `static/`). No build step. Charts via vendored Chart.js.

### Gotchas

- **Incremental indexing relies on file mtime.** If you manually edit a JSONL file, its mtime won't change unless you explicitly touch it. Use `./bin/tokenpulse index --rebuild` to force a full re-index when testing parser or indexer changes.
- **Cache create columns are split by TTL.** Pre-migration rows have only the legacy `cache_create_tokens` column; `splitCacheCreate()` treats unallocated tokens as 5m TTL. Run `index --rebuild` after a pricing update to backfill split columns and get accurate 1h pricing on old data.
- **Settings changes don't trigger a full reindex.** Changing pricing via the settings UI re-analyzes existing data but doesn't reparse JSONL files. Edit config.yaml directly and run `index --rebuild` if you need to resync pricing with historical logs.
- **SSE events drop on slow subscribers.** If a web client falls behind (slow network), its EventBus channel buffer fills and newer events are discarded. This is intentional (prevents blocking the reindex goroutine).

### Invariants worth preserving

- **Server binds to `127.0.0.1` only.** This is a personal-use tool; do not expose it to a network interface or add auth/multi-user concepts (see `.claude/PLAN.md` "Out of scope").
- **UTC by default.** All date bucketing uses `cfg.Location()`; users override via `timezone:` or `TP_TIMEZONE`.
- **Resume offsets matter.** `parser.ParseFile`'s rule that a partial line does not advance the offset is what makes incremental indexing correct under concurrent appends from Claude Code — preserve it.
- **No CGo.** `modernc.org/sqlite` was chosen for clean cross-compile (`CGO_ENABLED=0` in Makefile, GoReleaser, and CI). Do not introduce a CGo dependency.
- **Single writer discipline.** All store writes must go through methods that acquire `writeMu`. Long batches must split into ≤500-row transactions.

## Config & data paths

- Default Claude data: `~/.claude/projects/<slug>/<session>.jsonl`
- Default DB: `~/.config/tokenpulse/data.db`
- Override via `./config.yaml` (see `configs/config.yaml.example`) or `TP_*` env vars (e.g. `TP_SERVER_PORT=4000`, `TP_TIMEZONE=Asia/Tokyo`).

## CI / release

- CI matrix (`.github/workflows/ci.yml`): Ubuntu + macOS + Windows running `go vet`, `go test -race`, build, and `golangci-lint`. Note CI pins Go 1.22 while `go.mod` declares 1.25.1 — match the local toolchain to `go.mod`.
- Releases via GoReleaser (`.goreleaser.yaml`); Homebrew tap auto-published.
