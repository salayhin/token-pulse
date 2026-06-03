# Token Pulse Features Guide

A comprehensive overview of all features in Token Pulse.

## Dashboard Features

### 1. Overview Tab 📊

**Real-time Token & Cost Metrics**
- Today's usage vs. all-time totals
- Input, output, and cache tokens displayed
- Cost breakdown with "API Value" (cost at API rates)
- Support for flat-fee subscriptions (Pro, Max, Enterprise)

**Cache Analytics**
- Cache hit rate percentage
- Cache read tokens (M tokens)
- Cache create tokens (M tokens)
- Savings calculation based on tier (5m vs 1h TTL)

**Monthly Projections**
- Month-to-date spending
- 7-day daily average
- Projected full month cost
- Days remaining in month

**Cost Charts**
- 30-day trend chart with cost per day
- 7-day moving average overlay
- Interactive chart.js visualization
- Hover for daily details

**Model Breakdown**
- Input/output tokens per model
- Cost per model
- Model selection from dropdown

### 2. Projects Tab 🗂️

**Project List**
- All Claude Code projects with statistics
- Sessions count per project
- Total messages per project
- Total tokens per project
- Total cost per project
- Click-through to project sessions

**Project Search**
- Filter by project name/slug
- View project-specific stats

### 3. Sessions Tab 📝

**Session Browser**
- Complete list of all sessions
- Session title and git branch
- Start and end timestamps (in local timezone)
- Message count per session
- Cost per session (with API value equivalent)
- First prompt preview

**Session Filtering**
- Date range selection (from/to)
- Project filter
- Cursor-based pagination
- Limit control (25/50/100 sessions per page)

**Session Detail**
- Full message thread
- User vs. Assistant messages
- Model used for each response
- Tool calls invoked with names
- Token breakdown per turn
- Total session cost with API value

### 4. Budget Tab 💰

**Subscription Configuration**
- Plan selection: API, Pro, Max 5×, Max 20×, Team, Enterprise, Custom
- Monthly fee input (hidden for Enterprise)
- Real-time fee updates without restart

**Budget Management**
- Daily budget threshold
- Monthly budget threshold
- Automatic alerts when exceeded
- Budget vs. actual spending display

**Model Pricing**
- Input rate ($/1M tokens)
- Output rate ($/1M tokens)
- Cache read rate ($/1M tokens)
- Cache create 5m rate ($/1M tokens)
- Cache create 1h rate ($/1M tokens)
- Add/edit pricing for new models
- Fallback pricing for unknown models

**Plan-Specific Features**
- API: Show all pricing fields
- Pro/Max: Show monthly fee field
- Enterprise: Hide monthly fee, show API rates + seat price
- Team: Show per-seat pricing

### 5. Data Management Tab ⭐ NEW

**Rebuild Index Feature**
- One-click rebuild of entire index
- Re-import all Claude Code session data from local files
- Use when:
  - Pricing rates change (recalculate historical costs)
  - Database is out of sync with source data
  - Running first time (populate with data)

**Real-time Progress**
- Progress spinner during rebuild
- Live status messages
- File scan progress indicator
- Automatic close after completion (3s delay for visibility)

**Rebuild History Table**
- Shows last 10 rebuild attempts
- Columns: Status, Completed, Scanned, Indexed, Skipped, Messages, Tools, Duration
- Color-coded status:
  - Green for Success
  - Red for Failed
- Right-aligned numeric columns for easy scanning
- Sorted by most recent first

**Rebuild Statistics**
- Files scanned: Total files processed
- Files indexed: Files actually imported
- Files skipped: Files unchanged since last run
- Messages added: New messages imported
- Tool calls added: New tool calls tracked
- Duration: Time to complete rebuild
- Completed timestamp: When rebuild finished

**First Rebuild**
- "No rebuilds yet" message replaced with first row
- Automatic row addition without page refresh
- Immediate visual feedback

**Error Handling**
- Failed rebuilds appear in history
- Error message displayed in the row
- Red status indicator for failures
- Error stored for debugging

### 6. Settings Tab ⚙️

**Timezone Configuration**
- IANA timezone selector (all supported timezones)
- UTC always included
- Real-time updates to dashboard timestamps
- Local timezone displayed in top-right corner
- Affects all timestamps throughout app

**Subscription Plan**
- Plan selector dropdown
- Plan description and pricing model
- Monthly fee field (hidden for Enterprise)
- Real-time cost recalculation

**Model Pricing Editor**
- View current pricing for all models
- Add new model pricing
- Edit existing rates
- Table with columns: Model, Input, Output, Cache Read, Cache Cr 5m, Cache Cr 1h
- Add button to create new pricing entry
- Save/Cancel actions

**Configuration Path**
- Shows current config file location
- Helpful for troubleshooting

---

## Navigation Features

### Tab Navigation
- Top navigation bar with tabs
- Overview, Projects, Sessions, Budget, Data Management, Settings
- Active tab highlighted
- Hash-based routing (#overview, #sessions, etc.)
- Deep-linking supported

### Header Elements
- Logo/title clickable to go home
- Plan badge (shows if on flat-fee subscription)
- Timezone display (top-right corner)
- Live indicator dot (green = connected)
- Theme toggle (light/dark mode)

### Search & Filtering
- Session search by date range
- Project filtering
- Model selection
- Skill/tool filtering
- Full-text search in messages (via FTS5)

---

## Real-time Features

### Server-Sent Events (SSE)
- Live updates without polling
- Auto-refresh on session changes
- New session notifications
- Rebuild progress streaming
- Automatic reconnect on disconnect

### Auto-refresh
- After rebuild completes, dashboard auto-refreshes
- Overview stats update
- New sessions appear immediately
- Costs recalculated in real-time

---

## Data Export

### CSV Export
- Daily scope: Cost per day for 30+ days
- Sessions scope: All sessions with detailed columns
- Projects scope: Project-level statistics
- Open in Excel for analysis

### JSON Export
- Machine-readable format
- Preserves all data structure
- API values and raw calculations included
- Suitable for programmatic analysis

---

## Subscription Plans

### API
- Pay per token usage
- No monthly fee
- Display mode: shows API rates only

### Pro
- $20/month flat fee
- Variable usage overage
- Monthly fee field visible

### Max 5× & Max 20×
- $100-200/month flat fee
- Up to 5× or 20× base model limits
- Monthly fee field visible

### Team
- Per-seat pricing model
- Multiple team members
- Monthly fee field visible

### Enterprise
- Custom pricing with Anthropic
- Seat price + usage at API rates
- Monthly fee field hidden
- Display mode: shows "API Value" labels

### Custom
- Custom arrangement
- Monthly fee configurable
- Variable terms

---

## CLI Features

### Index Command
```bash
tokenpulse index              # Incremental index
tokenpulse index --rebuild    # Full rebuild
```

### Stats Command
```bash
tokenpulse stats              # All-time terminal stats
tokenpulse stats --today      # Today only stats
```

### Sessions Command
```bash
tokenpulse sessions list                          # All sessions
tokenpulse sessions list --project web-cli        # Filter by project
tokenpulse sessions list --limit 10               # Limit results
tokenpulse sessions show <session-id>             # Full session details
```

### Export Command
```bash
tokenpulse export --format csv --scope daily      # Daily CSV
tokenpulse export --format json --scope sessions  # Sessions JSON
tokenpulse export -o export.json                  # Custom output file
```

### Serve Command
```bash
tokenpulse serve              # Start dashboard (with startup index)
tokenpulse serve --skip-index # Start without indexing
```

---

## API Endpoints

### Statistics
- `GET /api/v1/stats` - Overall stats
- `GET /api/v1/stats/daily` - Daily breakdown
- `GET /api/v1/stats/trends` - Trend data
- `GET /api/v1/stats/projections` - Monthly projections

### Cache & Budget
- `GET /api/v1/cache` - Cache metrics
- `GET /api/v1/budget` - Budget status

### Projects & Sessions
- `GET /api/v1/projects` - All projects
- `GET /api/v1/projects/{slug}/stats` - Project stats
- `GET /api/v1/sessions` - All sessions (filterable)
- `GET /api/v1/sessions/{id}` - Session detail
- `GET /api/v1/sessions/{id}/skills` - Skills in session

### Metadata
- `GET /api/v1/models` - Model usage
- `GET /api/v1/prompts` - Prompt statistics
- `GET /api/v1/skills` - Skills breakdown

### Data Management
- `GET /api/v1/rebuild` - Rebuild history
- `POST /api/v1/rebuild` - Trigger rebuild
- `GET /api/v1/export` - Export data (CSV/JSON)

### Server
- `GET /api/v1/settings` - Current config
- `PUT /api/v1/settings` - Update config
- `GET /api/v1/events` - SSE stream
- `GET /api/v1/health` - Health check

---

## Keyboard Shortcuts

- Click logo → Go to Overview
- Click tab → Navigate to tab
- Click session → View details
- Click project → View sessions

---

## Theme Features

### Dark Mode
- Automatic theme detection (prefers-color-scheme)
- Manual toggle button (top-right)
- Preserves preference in session
- Better readability for different times of day

### Color Coding
- Success (green) for positive metrics
- Warning (orange) for alerts and limits
- Error (red) for failures and issues
- Muted text for secondary information

---

## Performance Features

- **Embedded SPA** - No loading delays for static files
- **SQLite with WAL** - Optimized for concurrent reads
- **FTS5 Search** - Instant message searching across 1000s of messages
- **Request Timeout** - 15s per API call (prevents hangs)
- **Streaming Export** - Large exports don't load into memory
- **Debounced Watcher** - 800ms debounce prevents excessive re-indexes
- **Incremental Indexing** - Only new bytes parsed after first run

---

Made with ❤️ for Claude Code users
