package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thassiov/engram-agent/internal/config"
)

const (
	notifyChannel = "engram_sync"

	// Push timing.
	pushBaseInterval = 30 * time.Second
	pushMaxInterval  = 2 * time.Minute

	// Pull reconnect timing.
	pullBaseDelay = 5 * time.Second
	pullMaxDelay  = 2 * time.Minute

	// Poll fallback when LISTEN is connected but as a safety net.
	pollInterval = 60 * time.Second
)

// backoff tracks an adaptive interval that grows on failure and resets on success.
type backoff struct {
	base    time.Duration
	max     time.Duration
	current time.Duration
}

func newBackoff(base, maxDelay time.Duration) *backoff {
	return &backoff{base: base, max: maxDelay, current: base}
}

// next returns the current delay with ±25% jitter, then doubles for next call.
func (b *backoff) next() time.Duration {
	d := b.current
	jitter := float64(d) * 0.25 * (2*rand.Float64() - 1)
	d += time.Duration(jitter)
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}
	return d
}

// reset returns to base interval after a success.
func (b *backoff) reset() {
	b.current = b.base
}

// Daemon runs the sync loop: pushes local mutations to PG and pulls remote
// mutations via LISTEN/NOTIFY with polling fallback.
type Daemon struct {
	cfg      *config.Config
	sqliteDB *sql.DB
	dsn      string
	logger   *slog.Logger
}

// NewDaemon creates a new sync daemon.
func NewDaemon(cfg *config.Config, sqliteDB *sql.DB, dsn string, logger *slog.Logger) *Daemon {
	return &Daemon{
		cfg:      cfg,
		sqliteDB: sqliteDB,
		dsn:      dsn,
		logger:   logger,
	}
}

// Run starts the daemon and blocks until the context is canceled.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Info("sync daemon starting", "machine", d.cfg.MachineID, "scope", d.cfg.Scope)

	go d.pushLoop(ctx)
	go WatchdogLoop(ctx)

	SDNotify("READY=1")

	d.pullLoop(ctx)

	SDNotify("STOPPING=1")

	return fmt.Errorf("sync daemon stopped: %w", ctx.Err())
}

func (d *Daemon) pushLoop(ctx context.Context) {
	bo := newBackoff(pushBaseInterval, pushMaxInterval)

	for {
		if ctx.Err() != nil {
			return
		}

		ok := d.doPush(ctx)
		if ok {
			bo.reset()
		}

		delay := bo.next()
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (d *Daemon) doPush(ctx context.Context) bool {
	conn, err := ConnectPG(ctx, d.dsn)
	if err != nil {
		d.logger.Warn("push: PG connect failed", "error", err)
		return false
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn.Close(closeCtx) //nolint:errcheck // best-effort cleanup
	}()

	n, seq, err := Push(ctx, d.sqliteDB, conn, d.cfg, d.logger)
	if err != nil {
		d.logger.Warn("push: error", "error", err)
		return false
	}
	if n > 0 {
		d.logger.Info("push: mutations pushed", "count", n, "cursor", seq)
	}
	return true
}

func (d *Daemon) pullLoop(ctx context.Context) {
	bo := newBackoff(pullBaseDelay, pullMaxDelay)

	for {
		if ctx.Err() != nil {
			return
		}

		start := time.Now()
		err := d.listenAndPull(ctx)
		if ctx.Err() != nil {
			return
		}

		if time.Since(start) > pollInterval {
			bo.reset()
		}

		delay := bo.next()
		d.logger.Info("pull: listener disconnected, reconnecting", "error", err, "delay", delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (d *Daemon) listenAndPull(ctx context.Context) error {
	conn, err := ConnectPG(ctx, d.dsn)
	if err != nil {
		return fmt.Errorf("PG connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn.Close(closeCtx) //nolint:errcheck // best-effort cleanup
	}()

	_, err = conn.Exec(ctx, "LISTEN "+notifyChannel)
	if err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}

	d.logger.Info("pull: listening", "channel", notifyChannel)

	if err := d.doPull(ctx, conn); err != nil {
		return fmt.Errorf("initial pull: %w", err)
	}

	for {
		waitCtx, cancel := context.WithTimeout(ctx, pollInterval)
		_, err := conn.WaitForNotification(waitCtx)
		timedOut := waitCtx.Err() == context.DeadlineExceeded
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("listener canceled: %w", ctx.Err())
			}
			if timedOut {
				if err := d.doPull(ctx, conn); err != nil {
					return fmt.Errorf("poll pull: %w", err)
				}
				continue
			}
			return fmt.Errorf("WaitForNotification: %w", err)
		}

		d.logger.Debug("pull: notification received")
		if err := d.doPull(ctx, conn); err != nil {
			return fmt.Errorf("notification pull: %w", err)
		}
	}
}

func (d *Daemon) doPull(ctx context.Context, conn *pgx.Conn) error {
	n, seq, err := Pull(ctx, conn, d.cfg, d.logger)
	if err != nil {
		d.logger.Warn("pull: error", "error", err)
		return err
	}
	if n > 0 {
		d.logger.Info("pull: mutations applied", "count", n, "cursor", seq)
	}
	return nil
}
