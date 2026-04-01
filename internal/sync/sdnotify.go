package sync

import (
	"context"
	"net"
	"os"
	"time"
)

// SDNotify sends a message to systemd's notification socket.
// Returns false if NOTIFY_SOCKET is not set (not running under systemd).
func SDNotify(state string) bool {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return false
	}

	conn, err := net.Dial("unixgram", sock)
	if err != nil {
		return false
	}
	defer conn.Close() //nolint:errcheck // best-effort notification

	_, err = conn.Write([]byte(state))
	return err == nil
}

// WatchdogLoop sends WATCHDOG=1 heartbeats to systemd at half the
// configured WatchdogSec interval. If WATCHDOG_USEC is not set, this
// is a no-op. Blocks until ctx is canceled.
func WatchdogLoop(ctx context.Context) {
	usecStr := os.Getenv("WATCHDOG_USEC")
	if usecStr == "" {
		return
	}

	var usec int64
	for _, c := range usecStr {
		if c < '0' || c > '9' {
			return
		}
		usec = usec*10 + int64(c-'0')
	}
	if usec <= 0 {
		return
	}

	// Ping at half the watchdog interval per sd_watchdog_enabled(3).
	interval := time.Duration(usec) * time.Microsecond / 2

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			SDNotify("WATCHDOG=1")
		}
	}
}
