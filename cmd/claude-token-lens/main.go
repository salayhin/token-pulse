package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirajus-salayhin/claude-token-lens/internal/alerts"
	"github.com/sirajus-salayhin/claude-token-lens/internal/analytics"
	"github.com/sirajus-salayhin/claude-token-lens/internal/config"
	"github.com/sirajus-salayhin/claude-token-lens/internal/indexer"
	"github.com/sirajus-salayhin/claude-token-lens/internal/server"
	"github.com/sirajus-salayhin/claude-token-lens/internal/server/handlers"
	"github.com/sirajus-salayhin/claude-token-lens/internal/store"
	"github.com/sirajus-salayhin/claude-token-lens/internal/watcher"
)

var (
	cfgFile  string
	logLevel string
)

func main() {
	root := &cobra.Command{
		Use:   "claude-token-lens",
		Short: "Local dashboard & CLI for Claude Code usage analytics",
	}
	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml or ~/.config/claude-token-lens/config.yaml)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "debug|info|warn|error")

	root.AddCommand(serveCmd())
	root.AddCommand(indexCmd())
	root.AddCommand(statsCmd())
	root.AddCommand(sessionsCmd())
	root.AddCommand(toolsCmd())
	root.AddCommand(exportCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newLogger() *slog.Logger {
	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(logLevel))
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func loadDeps() (*config.Config, *store.Store, *slog.Logger, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, nil, err
	}
	log := newLogger()
	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		return nil, nil, nil, err
	}
	st.WithLogger(log)
	return cfg, st, log, nil
}

func serveCmd() *cobra.Command {
	var skipIndex bool
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the local dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, log, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()

			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			if !skipIndex {
				log.Info("indexing on startup")
				idx := indexer.New(st, cfg.ClaudeDir, log)
				stats, err := idx.Run(ctx, false)
				if err != nil {
					log.Warn("startup index failed", "err", err)
				} else {
					log.Info("indexed",
						"files_scanned", stats.FilesScanned,
						"files_indexed", stats.FilesIndexed,
						"files_skipped", stats.FilesSkipped,
						"messages", stats.MessagesAdded,
						"tool_calls", stats.ToolCallsAdded,
						"dur", stats.Duration)
				}
			}

			eng := analytics.New(st.DB(), cfg)
			bus := handlers.NewEventBus()
			alertCheck := alerts.New(cfg, eng, log)
			alertCheck.Check(ctx)

			// Background file watcher → debounced re-index → SSE 'updated' events
			// → daily-budget alert check.
			idx := indexer.New(st, cfg.ClaudeDir, log)
			w := watcher.New(idx, alertingBus{bus, alertCheck, ctx}, cfg.ClaudeDir, log)
			go func() {
				if err := w.Run(ctx); err != nil {
					log.Warn("watcher exited", "err", err)
				}
			}()

			health := handlers.HealthInfo{
				DB:         st.DB(),
				InFlight:   w.InFlight,
				SlowWrites: st.SlowWrites,
			}
			s := server.New(cfg, eng, bus, health, log)
			return s.Start(ctx)
		},
	}
	cmd.Flags().BoolVar(&skipIndex, "skip-index", false, "skip the startup re-index")
	return cmd
}

func indexCmd() *cobra.Command {
	var rebuild bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Index ~/.claude into the local SQLite store",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, log, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			ctx := context.Background()
			idx := indexer.New(st, cfg.ClaudeDir, log)
			stats, err := idx.Run(ctx, rebuild)
			if err != nil {
				return err
			}
			fmt.Printf("scanned=%d indexed=%d skipped=%d messages=%d tool_calls=%d dur=%s\n",
				stats.FilesScanned, stats.FilesIndexed, stats.FilesSkipped,
				stats.MessagesAdded, stats.ToolCallsAdded, stats.Duration)
			return nil
		},
	}
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "force re-index of all files")
	return cmd
}

func statsCmd() *cobra.Command {
	var today bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Print usage stats to the terminal",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, _, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			eng := analytics.New(st.DB(), cfg)
			ctx := context.Background()
			s, err := eng.Stats(ctx)
			if err != nil {
				return err
			}
			cache, err := eng.Cache(ctx)
			if err != nil {
				return err
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer tw.Flush()

			t := s.AllTime
			label := "All time"
			if today {
				t = s.Today
				label = "Today"
			}
			fmt.Fprintf(tw, "claude-token-lens — %s (tz: %s)\n\n", label, s.Timezone)
			fmt.Fprintf(tw, "Sessions\t%d\n", t.Sessions)
			fmt.Fprintf(tw, "Messages\t%d\n", t.Messages)
			fmt.Fprintf(tw, "  assistant\t%d\n", t.AssistantMsgs)
			fmt.Fprintf(tw, "  user\t%d\n", t.UserMsgs)
			fmt.Fprintf(tw, "Tool calls\t%d\n", t.ToolCalls)
			fmt.Fprintf(tw, "Input tokens\t%d\n", t.InputTokens)
			fmt.Fprintf(tw, "Output tokens\t%d\n", t.OutputTokens)
			fmt.Fprintf(tw, "Cache create\t%d\n", t.CacheCreateTokens)
			fmt.Fprintf(tw, "Cache read\t%d\n", t.CacheReadTokens)
			fmt.Fprintf(tw, "Cost\t$%.2f\n", t.CostUSD)
			fmt.Fprintf(tw, "Net cache savings\t$%.2f\n", t.NetCacheSavingsUSD)
			fmt.Fprintf(tw, "\nCache hit rate\t%.1f%%\n", cache.HitRate*100)
			return nil
		},
	}
	cmd.Flags().BoolVar(&today, "today", false, "today only")
	return cmd
}

func sessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Browse and search sessions",
	}

	var listProject string
	var listLimit int
	listSub := &cobra.Command{
		Use:   "list",
		Short: "List recent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, _, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			eng := analytics.New(st.DB(), cfg)
			resp, err := eng.Sessions(context.Background(), listProject, "", listLimit)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer tw.Flush()
			fmt.Fprintln(tw, "ID\tPROJECT\tENDED\tMSGS\tCOST\tFIRST PROMPT")
			for _, s := range resp.Sessions {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t$%.2f\t%s\n",
					shortID(s.ID), s.ProjectSlug, shortTime(s.EndedAt),
					s.MessageCount, s.CostUSD, truncate(s.FirstPrompt, 60))
			}
			return nil
		},
	}
	listSub.Flags().StringVar(&listProject, "project", "", "filter by project slug")
	listSub.Flags().IntVar(&listLimit, "limit", 25, "max sessions to show")

	showSub := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show a session's message thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, _, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			eng := analytics.New(st.DB(), cfg)
			d, err := eng.Session(context.Background(), args[0])
			if err != nil {
				return err
			}
			if d == nil {
				return fmt.Errorf("session not found: %s", args[0])
			}
			fmt.Printf("Session %s  (project=%s, branch=%s)\n",
				d.Session.ID, d.Session.ProjectSlug, d.Session.GitBranch)
			fmt.Printf("Started %s · Ended %s · Messages %d · Cost $%.2f\n\n",
				shortTime(d.Session.StartedAt), shortTime(d.Session.EndedAt),
				d.Session.MessageCount, d.Session.CostUSD)
			for _, m := range d.Messages {
				if m.Role == "user-tool-result" {
					continue
				}
				prefix := strings.ToUpper(m.Role[:1]) + ":"
				if m.Role == "assistant" {
					prefix = fmt.Sprintf("A[%s]:", m.Model)
				}
				fmt.Printf("%s %s  %s\n", shortTime(m.Ts), prefix, truncate(m.Preview, 120))
				if len(m.ToolCalls) > 0 {
					names := make([]string, len(m.ToolCalls))
					for i, t := range m.ToolCalls {
						names[i] = t.Name
					}
					fmt.Printf("        ↳ tools: %s\n", strings.Join(names, ", "))
				}
			}
			return nil
		},
	}

	var searchProject string
	var searchLimit int
	searchSub := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across all messages",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, _, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			eng := analytics.New(st.DB(), cfg)
			q := strings.Join(args, " ")
			resp, err := eng.Search(context.Background(), q, searchProject, time.Time{}, time.Time{}, searchLimit)
			if err != nil {
				return err
			}
			fmt.Printf("Found %d hits in %s\n\n", resp.Total, resp.Took)
			for _, h := range resp.Hits {
				fmt.Printf("%s  [%s/%s] %s\n  %s\n\n",
					shortTime(h.Ts), h.ProjectSlug, h.Role, shortID(h.SessionID),
					stripMarkTags(h.Snippet))
			}
			return nil
		},
	}
	searchSub.Flags().StringVar(&searchProject, "project", "", "filter by project slug")
	searchSub.Flags().IntVar(&searchLimit, "limit", 20, "max hits to show")

	cmd.AddCommand(listSub, showSub, searchSub)
	return cmd
}

func toolsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tools", Short: "Tool call analytics"}
	cmd.AddCommand(&cobra.Command{
		Use:   "show <tool-name>",
		Short: "Per-tool stats",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, _, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			eng := analytics.New(st.DB(), cfg)
			d, err := eng.ToolDetail(context.Background(), args[0])
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(d, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})
	return cmd
}

func exportCmd() *cobra.Command {
	var format, scope, outPath string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export analytics as CSV or JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, st, _, err := loadDeps()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			eng := analytics.New(st.DB(), cfg)

			var w *os.File
			if outPath == "" || outPath == "-" {
				w = os.Stdout
			} else {
				w, err = os.Create(outPath)
				if err != nil {
					return err
				}
				defer w.Close()
			}
			return eng.Export(context.Background(),
				analytics.ExportScope(scope),
				analytics.ExportFormat(format), w)
		},
	}
	cmd.Flags().StringVar(&format, "format", "csv", "csv|json")
	cmd.Flags().StringVar(&scope, "scope", "daily", "daily|sessions|tools|projects")
	cmd.Flags().StringVarP(&outPath, "output", "o", "-", "output file (- for stdout)")
	return cmd
}

// alertingBus wraps the SSE event bus so each published 'updated' event also
// re-checks today's spend against the daily budget.
type alertingBus struct {
	inner   *handlers.EventBus
	checker *alerts.Checker
	ctx     context.Context
}

func (a alertingBus) Publish(t string, data any) {
	a.inner.Publish(t, data)
	if t == "updated" {
		a.checker.Check(a.ctx)
	}
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
func shortTime(s string) string {
	if len(s) >= 16 {
		return s[:16]
	}
	return s
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
func stripMarkTags(s string) string {
	s = strings.ReplaceAll(s, "<mark>", "**")
	s = strings.ReplaceAll(s, "</mark>", "**")
	return s
}
