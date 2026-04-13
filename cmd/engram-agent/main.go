package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/thassiov/engram-agent/internal/config"
	"github.com/thassiov/engram-agent/internal/embed"
	"github.com/thassiov/engram-agent/internal/extract"
	"github.com/thassiov/engram-agent/internal/server"
	"github.com/thassiov/engram-agent/internal/state"
	"github.com/thassiov/engram-agent/internal/sync"

	_ "modernc.org/sqlite"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
	// BuildTime is set at build time via ldflags.
	BuildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var configPath string

	rootCmd := &cobra.Command{
		Use:           "engram-agent",
		Short:         "Observation extraction, embedding, and sync agent for engram",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", config.DefaultConfigPath(), "path to config file")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDaemonCmd(&configPath))
	rootCmd.AddCommand(newStatusCmd(&configPath))
	rootCmd.AddCommand(newBackfillCmd(&configPath))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return rootCmd.ExecuteContext(ctx)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("engram-agent version %s (built %s)\n", Version, BuildTime)
		},
	}
}

func newDaemonCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the agent daemon (sync + hook listener)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			logger.Info("starting engram-agent",
				"version", Version,
				"machine", cfg.MachineID,
				"scope", cfg.Scope,
			)

			// Open internal state DB.
			stateDB, err := state.Open(state.DefaultPath())
			if err != nil {
				return fmt.Errorf("opening state DB: %w", err)
			}
			defer stateDB.Close()

			// Start sync daemon if PG is configured.
			if cfg.SyncEnabled() {
				sqliteDB, err := openSQLite(cfg.EngramDB)
				if err != nil {
					return fmt.Errorf("opening engram DB: %w", err)
				}
				defer sqliteDB.Close()

				dsn := cfg.Postgres.DSN()
				syncDaemon := sync.NewDaemon(cfg, sqliteDB, dsn, logger)
				syncDaemon.SetStateDB(stateDB.RawDB())

				go func() {
					if err := syncDaemon.Run(cmd.Context()); err != nil {
						logger.Error("sync daemon stopped", "error", err)
					}
				}()
			} else {
				logger.Info("sync disabled (no postgres config)")
			}

			// Create extraction watcher.
			watcher := extract.NewWatcher(stateDB, cfg.OllamaURL, cfg.OllamaFallbackURL, cfg.OllamaModel, cfg.EngramAPI, cfg.EmbedURL, cfg.DedupThreshold, logger)

			// Create search handler if embedding is configured.
			var searchHandler *server.SearchHandler
			if cfg.EmbedURL != "" {
				embedClient := embed.New(cfg.EmbedURL)
				searchHandler = server.NewSearchHandler(stateDB, embedClient, logger)
				if err := searchHandler.RefreshCache(); err != nil {
					logger.Warn("initial vector cache load failed", "error", err)
				}
			}

			// Start HTTP hook listener.
			ctx := cmd.Context()
			srv := server.New(cfg.ListenAddr, func(n server.Notification) {
				watcher.HandleNotification(ctx, n.SessionID, n.Event, n.Reset)
			}, searchHandler, logger)

			go func() {
				if err := srv.ListenAndServe(); err != nil {
					logger.Error("hook listener failed", "error", err)
				}
			}()

			// Block until shutdown signal.
			<-cmd.Context().Done()
			logger.Info("shutting down")
			return nil
		},
	}
}

func newStatusCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show sync status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			fmt.Printf("Machine:     %s\n", cfg.MachineID)
			fmt.Printf("Scope:       %s\n", cfg.Scope)
			fmt.Printf("Engram DB:   %s\n", cfg.EngramDB)
			fmt.Printf("Engram API:  %s\n", cfg.EngramAPI)
			fmt.Printf("Listen:      %s\n", cfg.ListenAddr)
			fmt.Printf("Ollama:      %s (%s)\n", cfg.OllamaURL, cfg.OllamaModel)
			if cfg.OllamaFallbackURL != "" {
				fmt.Printf("Ollama (fb): %s\n", cfg.OllamaFallbackURL)
			}

			if cfg.PullsAll() {
				fmt.Println("Pull filter: all")
			} else {
				fmt.Printf("Pull filter: %v\n", cfg.PullFilterTypes())
			}

			// Show push cursor.
			cursorPath := sync.PushCursorFile()
			seq, err := sync.ReadPushCursor(cursorPath)
			if err != nil {
				fmt.Printf("Push cursor: error (%v)\n", err)
			} else {
				fmt.Printf("Push cursor: %d\n", seq)
			}

			// PG sync status.
			if !cfg.SyncEnabled() {
				fmt.Println("Sync:        disabled (no postgres config)")
				return nil
			}

			dsn := cfg.Postgres.DSN()
			pgConn, err := sync.ConnectPG(cmd.Context(), dsn)
			if err != nil {
				fmt.Printf("PG status:   unreachable (%v)\n", err)
				return nil
			}
			defer pgConn.Close(cmd.Context()) //nolint:errcheck

			pullSeq, err := sync.ReadCursor(cmd.Context(), pgConn, cfg.MachineID)
			if err != nil {
				fmt.Printf("Pull cursor: error (%v)\n", err)
			} else {
				fmt.Printf("Pull cursor: %d\n", pullSeq)
			}

			var count int64
			err = pgConn.QueryRow(cmd.Context(), "SELECT COUNT(*) FROM engram_sync_mutations").Scan(&count)
			if err != nil {
				fmt.Printf("PG mutations: error (%v)\n", err)
			} else {
				fmt.Printf("PG mutations: %d\n", count)
			}

			return nil
		},
	}
}

func newBackfillCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "backfill",
		Short: "Generate embeddings for all engram.db observations missing vectors",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			if cfg.EmbedURL == "" {
				return fmt.Errorf("embed_url not configured")
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			// Open state DB (where vectors are stored).
			stateDB, err := state.Open(state.DefaultPath())
			if err != nil {
				return fmt.Errorf("opening state DB: %w", err)
			}
			defer stateDB.Close()

			// Open engram.db (read-only, source of all observations).
			engramDB, err := openSQLite(cfg.EngramDB)
			if err != nil {
				return fmt.Errorf("opening engram DB: %w", err)
			}
			defer engramDB.Close()

			embedClient := embed.New(cfg.EmbedURL)
			if !embedClient.Reachable(cmd.Context()) {
				return fmt.Errorf("embedding service at %s is unreachable", cfg.EmbedURL)
			}

			// Get IDs already backfilled.
			existing, err := stateDB.EngramVectorIDs()
			if err != nil {
				return fmt.Errorf("querying existing vectors: %w", err)
			}

			// Read all observations from engram.db.
			rows, err := engramDB.Query(`
				SELECT id, title, COALESCE(content, ''), type, COALESCE(scope, 'project'),
				       COALESCE(project, 'general'), COALESCE(topic_key, '')
				FROM observations
				WHERE deleted_at IS NULL
			`)
			if err != nil {
				return fmt.Errorf("querying engram observations: %w", err)
			}
			defer rows.Close()

			var backfilled, skipped int
			for rows.Next() {
				var (
					id                                              int64
					title, content, obsType, scope, project, topicKey string
				)
				if err := rows.Scan(&id, &title, &content, &obsType, &scope, &project, &topicKey); err != nil {
					logger.Error("scanning observation", "error", err)
					continue
				}

				if existing[id] {
					skipped++
					continue
				}

				text := title + "\n" + content
				vec, err := embedClient.EmbedOne(cmd.Context(), text)
				if err != nil {
					logger.Error("failed to embed", "id", id, "error", err)
					continue
				}

				if err := stateDB.SaveEngramVector(id, title, content, obsType, scope, project, topicKey, vec); err != nil {
					logger.Error("failed to save vector", "id", id, "error", err)
					continue
				}

				backfilled++
				if backfilled%50 == 0 {
					logger.Info("progress", "backfilled", backfilled)
				}
			}

			fmt.Printf("Backfill complete: %d embedded, %d already existed.\n", backfilled, skipped)
			return nil
		},
	}
}

func openSQLite(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening SQLite %s: %w", path, err)
	}

	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting busy_timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode = wal"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting journal_mode: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging SQLite %s: %w", path, err)
	}
	return db, nil
}
