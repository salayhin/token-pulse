# Token Pulse

> A personal-use local dashboard and CLI for monitoring Claude Code token usage, costs, and cache savings.
>
> **Single binary • Embedded UI • Zero external dependencies • No multi-user auth**

Token Pulse is a Go application that monitors your Claude Code sessions locally. It reads your session data from `~/.claude/projects/`, indexes it into SQLite, and serves a real-time dashboard at `http://127.0.0.1:3456`.

---

## 🚀 Quick Start

### Option 1: One-line startup (recommended)

```bash
./build-and-run.sh
# Opens http://127.0.0.1:3456 automatically
```

The script:
- Kills any existing process on port 3456
- Builds the binary
- Indexes all existing sessions
- Starts the dashboard server

### Option 2: Manual steps

```bash
# Build the binary
make build

# One-time index of all existing sessions
./bin/tokenpulse index

# View stats in the terminal
./bin/tokenpulse stats --today

# Start the dashboard server
./bin/tokenpulse serve
# Open http://127.0.0.1:3456 in your browser
```

---

## ✨ Features

### 📊 Dashboard Tabs

#### **Overview**
- Real-time token usage (input/output/cache read/create)
- Today vs. all-time cost breakdown
- Cache hit rate and volume metrics
- 30-day cost trend chart with 7-day moving average
- Monthly projection and remaining budget
- Cost per model breakdown

#### **Projects**
- List all Claude Code projects with stats
- Total sessions, messages, and tokens per project
- Project-level cost tracking
- Click through to project-specific sessions

#### **Sessions**
- Browse all sessions with full search and filtering
- Date range filtering (from/to)
- Per-session token breakdown
- Cost per session with API value equivalent
- Session title, git branch, and message count
- Deep-dive into individual sessions with full message thread

#### **Budget**
- Configure subscription plan (API, Pro, Max, Enterprise, Custom, Team)
- Set monthly fee and budget limits
- Model-specific pricing rates (input, output, cache read, cache create)
- Real-time cost tracking against budget
- Daily pace calculator

#### **Data Management** ⭐ *NEW*
- **Rebuild Index**: Re-import all Claude Code session data from local files
- **Live Progress**: Real-time progress feedback during rebuild with file scan status
- **Rebuild History**: Complete history table showing all rebuild attempts (last 10)
- **Instant Feedback**: Rows appear immediately in the table as rebuilds complete
- **Error Tracking**: Failed rebuilds appear with error details
- **Stats Tracking**: Each rebuild shows:
  - Files scanned, indexed, skipped
  - Messages and tool calls added
  - Duration and completion timestamp
  - Color-coded status (green for success, red for failed)

#### **Settings**
- Live timezone selection with UTC support
- Real-time pricing updates (no restart required)
- Subscription plan configuration
- Model pricing editor (add/edit pricing for any Claude model)
- CSV export of all settings

### 🎯 Key Capabilities

- **Real-time Updates** — SSE (Server-Sent Events) streaming for live data
- **Session Explorer** — Browse messages, tool calls, and tokens per turn
- **Model Analytics** — Input/output distribution across all models
- **Cache Insights** — Detailed cache hit rates, savings calculations
- **Timezone Support** — View all timestamps in your local timezone
- **Data Export** — CSV or JSON export for daily stats, sessions, or projects
- **Live Search** — Full-text search across session messages
- **Responsive Design** — Mobile-friendly dashboard UI
- **Theme Toggle** — Light/dark mode support

---

## 🏗️ System Architecture

```
~/.claude/projects/<slug>/<session>.jsonl
        │
   parser  ──► indexer ──► store (SQLite + FTS5)
                              │
                              ├─► analytics ──► server (chi REST + SSE)
                              │                       │
                              │                       └─► embedded SPA (web/)
                              └─► analytics ──► CLI (cobra)

   watcher (fsnotify) ──► debounced reindex ──► EventBus.Publish("updated") 
                                                  ├─► SSE clients
                                                  └─► alerts.Check (daily budget)
```

### Data Flow

1. **Source** — Claude Code writes JSONL files to `~/.claude/projects/<slug>/<session-uuid>.jsonl`
2. **Parser** — Converts raw JSONL into typed structs (messages, tool calls, usage)
3. **Indexer** — Walks projects directory, incremental or full rebuild
4. **Store** — SQLite database with FTS5 full-text search
5. **Analytics** — Read-only queries with cost/cache calculations
6. **Watcher** — Auto-detects new sessions and re-indexes incrementally
7. **Server** — REST API + SSE for real-time dashboard
8. **UI** — Embedded SPA (no build step required)

---

## 📋 Configuration

### Config File Location (Lookup Order)
1. `./config.yaml` (current directory)
2. `~/.config/tokenpulse/config.yaml` (user config)
3. Environment variables (`TP_*`)
4. Built-in defaults

### Example Config
```yaml
# Server
server:
  host: 127.0.0.1      # Only localhost for security
  port: 3456

# Data
storage:
  path: ~/.config/tokenpulse/data.db

# Timezone
timezone: Asia/Tokyo

# Claude directory
claude_dir: ~/.claude

# Subscription
subscription:
  plan: api             # api | pro | max_5x | max_20x | team | enterprise | custom
  monthly_fee_usd: 0

# Daily budget (alerts when exceeded)
budget_usd: 100

# Model pricing ($/1M tokens)
pricing:
  models:
    claude-sonnet-4:
      input: 3.0
      output: 15.0
      cache_read: 0.3
      cache_create_5m: 3.75
      cache_create_1h: 0.30
  fallback:
    input: 3.0
    output: 15.0

# Alerts
alerts:
  notify: false         # macOS notifications (requires osascript)
```

### Environment Variables
```bash
TP_SERVER_PORT=4000
TP_TIMEZONE=Europe/London
TP_BUDGET_USD=200
TP_STORAGE_PATH=/custom/path/data.db
```

---

## 💾 Data Management

### Incremental Indexing (Default)
- Automatic on startup and via file watcher
- Skips unchanged files (by size + mtime)
- Resumes from last byte offset
- **Cost**: O(new bytes) per watcher tick

### Full Rebuild
Trigger a full re-index when:
- Pricing rates change (to recalculate all historical costs)
- Database becomes out of sync with source files
- Cache TTL split columns need backfilling

**Via Dashboard:**
1. Click the "Data Management" tab
2. Click "Rebuild Index" button
3. Watch real-time progress
4. View rebuild history with statistics

**Via CLI:**
```bash
./bin/tokenpulse index --rebuild
```

### Cache Savings Calculation
Cache creation tokens are split by TTL because Anthropic charges different rates:

```
gross_saved   = cache_read_tokens × (input_rate − cache_read_rate)
extra_paid_5m = cache_create_5m   × (cache_create_5m_rate − input_rate)
extra_paid_1h = cache_create_1h   × (cache_create_1h_rate − input_rate)
net_saved     = gross_saved − extra_paid_5m − extra_paid_1h
```

---

## 🛠️ CLI Commands

```bash
# Index
./bin/tokenpulse index                    # Incremental index
./bin/tokenpulse index --rebuild          # Full rebuild

# Stats
./bin/tokenpulse stats                    # All-time stats
./bin/tokenpulse stats --today            # Today only

# Sessions
./bin/tokenpulse sessions list            # List all sessions
./bin/tokenpulse sessions list --project web-cli
./bin/tokenpulse sessions show <session-id>

# Dashboard
./bin/tokenpulse serve                    # Start dashboard
./bin/tokenpulse serve --skip-index       # Skip startup index

# Export
./bin/tokenpulse export --format csv --scope daily
./bin/tokenpulse export --format json --scope sessions -o export.json
```

---

## 📦 Installation

### From Source
```bash
git clone https://github.com/salayhin/token-pulse.git
cd token-pulse
make build
./bin/tokenpulse serve
```

### From Homebrew (macOS/Linux)
```bash
brew tap salayhin/tokenpulse
brew install tokenpulse
tokenpulse serve
```

### From Release Binary
Download pre-built binaries from [releases](https://github.com/salayhin/token-pulse/releases) for:
- macOS (Intel, Apple Silicon)
- Linux (x86_64)
- Windows

---

## 🔧 Development

### Build & Test
```bash
make build              # Build binary → bin/tokenpulse
make run                # go run ./cmd/tokenpulse serve
make test               # Run all tests with race detection
make lint               # golangci-lint
make clean              # Clean build artifacts
```

### Project Structure
```
token-pulse/
├── cmd/tokenpulse/          # CLI entrypoint (cobra)
├── internal/
│   ├── parser/              # JSONL → Record conversion
│   ├── indexer/             # File walking & incremental parsing
│   ├── store/               # SQLite schema & operations
│   ├── analytics/           # Cost & cache calculations
│   ├── server/              # HTTP router & handlers
│   ├── config/              # Viper config management
│   ├── watcher/             # fsnotify listener
│   └── alerts/              # Budget checking & notifications
├── web/                      # Embedded SPA (index.html + static/)
├── configs/                  # Example configuration
└── Makefile
```

### Key Design Decisions

1. **No CGo** — Uses `modernc.org/sqlite` for clean cross-compilation
2. **Embedded SPA** — No build step, no Node.js dependency
3. **Single Writer** — Mutex-protected SQLite writes, batched ≤500 rows per transaction
4. **Resume-Safe Parser** — Partial lines never advance offset, critical for incremental indexing under concurrent Claude Code appends
5. **127.0.0.1 Only** — Personal-use tool, no multi-user auth, no network exposure

---

## 📊 API Endpoints

All endpoints are prefixed with `/api/v1`:

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/stats` | Overall statistics |
| GET | `/stats/daily` | Daily breakdown (last 30 days) |
| GET | `/stats/trends` | Trend data for charting |
| GET | `/stats/projections` | Monthly projections |
| GET | `/cache` | Cache hit rate and metrics |
| GET | `/budget` | Budget status and alerts |
| GET | `/skills` | Skills/plugins breakdown |
| GET | `/projects` | All projects |
| GET | `/projects/{slug}/stats` | Project-specific stats |
| GET | `/sessions` | All sessions (with filtering) |
| GET | `/sessions/{id}` | Session detail with message thread |
| GET | `/sessions/{sessionId}/skills` | Skills used in session |
| GET | `/prompts` | Prompt statistics |
| GET | `/models` | Model usage breakdown |
| GET | `/settings` | Current configuration |
| GET | `/rebuild` | Rebuild history |
| POST | `/rebuild` | Trigger index rebuild |
| PUT | `/settings` | Update configuration |
| GET | `/export` | Export data (CSV/JSON) |
| GET | `/events` | SSE stream for real-time updates |
| GET | `/health` | Server health & database stats |

---

## ⚙️ Requirements

- **Go 1.25.1+** (for building from source)
- **macOS, Linux, or Windows**
- **Your Claude Code session directory** (`~/.claude/projects/`)

No other dependencies! Everything is statically compiled.

---

## 📝 License

MIT License — See LICENSE file

---

## 🤝 Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit changes (`git commit -am 'Add my feature'`)
4. Push to branch (`git push origin feature/my-feature`)
5. Open a Pull Request

## ⚡ Performance Notes

- **SQLite WAL Mode** — Optimized for concurrent reads
- **FTS5 Full-Text Search** — Instant message searching
- **Incremental Indexing** — Only new bytes are parsed (O(n) on new data)
- **Debounced Watcher** — 800ms debounce prevents excessive re-indexes
- **Request Timeout** — 15s per bounded API request (prevents hangs)
- **Streaming Export** — Large exports don't load into memory

---

## 🐛 Troubleshooting

### "No rebuilds yet" on Data Management tab
- This is normal on first startup
- Click "Rebuild Index" to populate the rebuild history

### Timestamps showing in UTC instead of local timezone
- Check your config: `timezone: Asia/Tokyo`
- Or set env var: `TP_TIMEZONE=Asia/Tokyo`
- Restart the server

### Database file not found
- Check `storage.path` in config (default: `~/.config/tokenpulse/data.db`)
- Run `./bin/tokenpulse index` to create it

### Rebuild takes a long time
- This is normal for first rebuild on large datasets
- Subsequent rebuilds are much faster (incremental)
- Progress shows in the Data Management tab

---

## 📊 Sample Dashboard

**Overview Tab:**
- Today's cost: $12.34
- All-time cost: $245.67
- Cache hit rate: 42.3%
- 30-day trend chart
- Monthly projection

**Data Management Tab:**
- Rebuild button with live progress
- Rebuild history showing last 10 rebuilds
- Each rebuild row shows: Status, Completed time, Files scanned/indexed, Messages, Tools, Duration

**Sessions Tab:**
- 156 total sessions
- Search/filter by date, project, cost
- Deep dive into individual sessions

---

Made with ❤️ for Claude Code users tracking their token usage locally.
