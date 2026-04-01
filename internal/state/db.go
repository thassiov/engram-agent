// Package state manages the internal SQLite state database for session tracking,
// chunks, and extracted observations.
package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the internal state database.
type DB struct {
	db *sql.DB
}

// DefaultPath returns the default state DB path.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "engram-agent", "state.db")
}

// Open opens (or creates) the state database.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening state DB: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting busy_timeout: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating state DB: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS session_state (
			session_id   TEXT PRIMARY KEY,
			session_log  TEXT NOT NULL,
			last_turn    INTEGER NOT NULL DEFAULT 0,
			status       TEXT NOT NULL DEFAULT 'active',
			created_at   TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS chunks (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id   TEXT NOT NULL,
			batch_id     INTEGER NOT NULL,
			turn_start   INTEGER NOT NULL,
			turn_end     INTEGER NOT NULL,
			content      TEXT,
			char_count   INTEGER NOT NULL DEFAULT 0,
			status       TEXT NOT NULL DEFAULT 'pending',
			created_at   TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS observations (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id   TEXT NOT NULL,
			chunk_id     INTEGER REFERENCES chunks(id),
			type         TEXT NOT NULL,
			title        TEXT NOT NULL,
			content      TEXT,
			scope        TEXT NOT NULL DEFAULT 'project',
			project      TEXT NOT NULL DEFAULT 'general',
			topic_key    TEXT,
			status       TEXT NOT NULL DEFAULT 'pending',
			created_at   TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS vectors (
			observation_id INTEGER PRIMARY KEY REFERENCES observations(id),
			embedding      BLOB NOT NULL,
			dims           INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS logs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp    TEXT NOT NULL DEFAULT (datetime('now')),
			level        TEXT NOT NULL DEFAULT 'info',
			component    TEXT,
			session_id   TEXT,
			message      TEXT NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("creating tables: %w", err)
	}
	return nil
}
