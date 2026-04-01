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

			// Open engram's SQLite DB (read-only for sync).
			sqliteDB, err := openSQLite(cfg.EngramDB)
			if err != nil {
				return fmt.Errorf("opening engram DB: %w", err)
			}
			defer sqliteDB.Close()

			// Open internal state DB.
			stateDB, err := state.Open(state.DefaultPath())
			if err != nil {
				return fmt.Errorf("opening state DB: %w", err)
			}
			defer stateDB.Close()

			// Build PG DSN.
			dsn := sync.BuildDSN(cfg.PGCredentials)

			// Create sync daemon.
			syncDaemon := sync.NewDaemon(cfg, sqliteDB, dsn, logger)

			// Create extraction watcher.
			watcher := extract.NewWatcher(stateDB, cfg.OllamaURL, cfg.OllamaModel, cfg.EngramAPI, logger)

			// Start HTTP hook listener.
			ctx := cmd.Context()
			srv := server.New(cfg.ListenAddr, func(n server.Notification) {
				watcher.HandleNotification(ctx, n.SessionID, n.Event)
			}, logger)

			go func() {
				if err := srv.ListenAndServe(); err != nil {
					logger.Error("hook listener failed", "error", err)
				}
			}()

			// Run sync daemon (blocks until context is canceled).
			return syncDaemon.Run(cmd.Context())
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

			// Try PG connection for pull cursor.
			dsn := sync.BuildDSN(cfg.PGCredentials)
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

			// Count PG mutations.
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
