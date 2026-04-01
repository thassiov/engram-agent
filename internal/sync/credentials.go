// Package sync provides cross-machine observation sync via PostgreSQL.
package sync

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

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
