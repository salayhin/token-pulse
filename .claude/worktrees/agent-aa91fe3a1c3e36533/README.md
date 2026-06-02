# tokenpulse

A Go-powered local dashboard and CLI for monitoring Claude Code usage.
Single binary, embedded UI, no external dependencies.

> Personal use only. Server binds to `127.0.0.1` and reads your local
> `~/.claude/projects/` JSONL files. No multi-user, no auth, no upload.

## Quick start

```bash
make build
./bin/tokenpulse index           # one-time ingest into SQLite
./bin/tokenpulse stats           # all-time
./bin/tokenpulse stats --today   # today only
./bin/tokenpulse serve           # dashboard at http://127.0.0.1:3456
```

## Configuration

Defaults are sensible for personal use. To override, copy
`configs/config.yaml.example` to `./config.yaml` (or
`~/.config/tokenpulse/config.yaml`) and edit. Env vars (`TP_*`)
also work, e.g. `TP_TIMEZONE=Asia/Tokyo`.

## Data model

- `~/.claude/projects/<slug>/<session-uuid>.jsonl` — Claude Code's session log
- Indexed once into `~/.config/tokenpulse/data.db`
- Re-running `index` is incremental (skips files unchanged by size+mtime)

## Cache savings math

Claude's prompt cache makes some reads cheaper but charges a creation
premium. The headline "savings" number is the **net** of both:

```
gross_saved = cache_read_tokens   × (input_rate − cache_read_rate)
extra_paid  = cache_create_tokens × (cache_create_rate − input_rate)
net_saved   = gross_saved − extra_paid
```

## See also

- See `.claude/PLAN.md` for the full roadmap (Phase 0 → 3)
- See `.claude/PLAN_REVIEW.md` for the audit trail
