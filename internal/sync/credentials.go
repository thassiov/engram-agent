// Package sync provides cross-machine observation sync via PostgreSQL.
package sync

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// BuildDSN constructs a PostgreSQL DSN using a credential chain:
// env vars → credentials file.
func BuildDSN(credentialsFile string) string {
	if dsn := os.Getenv("ENGRAM_SYNC_DSN"); dsn != "" {
		return dsn
	}
	if dsn := os.Getenv("KB_DSN"); dsn != "" {
		return dsn
	}

	host := envOr("POSTGRES_HOST", "postgres.grid.local")
	port := envOr("POSTGRES_PORT", "5432")
	database := envOr("KB_DATABASE", "knowledge")
	user := envOr("POSTGRES_USER", "grid_admin")
	sslmode := envOr("POSTGRES_SSLMODE", "disable")

	password := envFirst("POSTGRES_PASSWORD", "POSTGRES_PASS")

	if password == "" {
		password = loadPasswordFromFile(credentialsFile)
	}
	if password == "" {
		password = loadPasswordFromGridSecrets()
	}

	return fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s"+
			" tcp_user_timeout=30000", // 30s — fail fast on unacked data.
		host, port, database, user, password, sslmode)
}

// ConnectPG parses the DSN and connects with client-side TCP keepalive.
func ConnectPG(ctx context.Context, dsn string) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing DSN: %w", err)
	}

	cfg.Config.AfterNetConnect = func(_ context.Context, _ *pgconn.Config, conn net.Conn) (net.Conn, error) {
		tc, ok := conn.(*net.TCPConn)
		if !ok {
			return conn, nil // unix socket, nothing to do
		}
		if err := tc.SetKeepAliveConfig(net.KeepAliveConfig{
			Enable:   true,
			Idle:     60 * time.Second, // 60s before first probe
			Interval: 15 * time.Second, // 15s between probes
			Count:    4,                // 4 missed probes → dead
		}); err != nil {
			return nil, fmt.Errorf("setting TCP keepalive: %w", err)
		}
		return conn, nil
	}

	return pgx.ConnectConfig(ctx, cfg)
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func loadPasswordFromFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key == "POSTGRES_PASSWORD" || key == "POSTGRES_PASS" {
			return value
		}
	}
	return ""
}

// loadPasswordFromGridSecrets tries to get the PG password via the grid CLI.
func loadPasswordFromGridSecrets() string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "grid", "secrets", "get", "postgres-grid-admin", "password").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
