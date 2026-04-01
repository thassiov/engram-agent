package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thassiov/engram-agent/internal/config"
)

// Mutation represents a row from engram's sync_mutations table.
type Mutation struct {
	Seq        int64
	TargetKey  string
	Entity     string
	EntityKey  string
	Op         string
	Payload    string
	Source     string
	Project    string
	OccurredAt string
}

// PushCursorFile returns the path to the local push cursor file.
func PushCursorFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "engram", "push-cursor")
}

// ReadPushCursor reads the last pushed seq from the local cursor file.
func ReadPushCursor(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading push cursor: %w", err)
	}
	seq, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing push cursor: %w", err)
	}
	return seq, nil
}

// WritePushCursor writes the push cursor to the local file.
func WritePushCursor(path string, seq int64) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating cursor dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strconv.FormatInt(seq, 10)), 0o644); err != nil {
		return fmt.Errorf("writing push cursor: %w", err)
	}
	return nil
}

// ReadMutationsFromSQLite reads new mutations from engram's local SQLite database.
func ReadMutationsFromSQLite(db *sql.DB, afterSeq int64) ([]Mutation, error) {
	rows, err := db.Query(`
		SELECT seq, target_key, entity, entity_key, op, payload, source, project, occurred_at
		FROM sync_mutations
		WHERE seq > ?
		ORDER BY seq ASC
	`, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("querying sync_mutations: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var mutations []Mutation
	for rows.Next() {
		var m Mutation
		if err := rows.Scan(&m.Seq, &m.TargetKey, &m.Entity, &m.EntityKey, &m.Op, &m.Payload, &m.Source, &m.Project, &m.OccurredAt); err != nil {
			return nil, fmt.Errorf("scanning mutation: %w", err)
		}
		mutations = append(mutations, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating mutations: %w", err)
	}
	return mutations, nil
}

// Push reads new mutations from local SQLite and inserts them into PostgreSQL.
// Returns the number of mutations pushed and the new high-water mark.
func Push(ctx context.Context, sqliteDB *sql.DB, pgConn *pgx.Conn, cfg *config.Config, logger *slog.Logger) (pushed int, highWater int64, err error) {
	cursorPath := PushCursorFile()
	lastSeq, err := ReadPushCursor(cursorPath)
	if err != nil {
		return 0, 0, err
	}

	mutations, err := ReadMutationsFromSQLite(sqliteDB, lastSeq)
	if err != nil {
		return 0, lastSeq, err
	}

	if len(mutations) == 0 {
		return 0, lastSeq, nil
	}

	tx, err := pgConn.Begin(ctx)
	if err != nil {
		return 0, lastSeq, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is best-effort after commit

	var maxSeq int64
	for i := range mutations {
		m := &mutations[i]
		scope := scopeFromMutation(*m, cfg)
		occurredAt, _ := time.Parse("2006-01-02 15:04:05", m.OccurredAt)
		if occurredAt.IsZero() {
			occurredAt = time.Now()
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO engram_sync_mutations
				(source_seq, source_machine, entity, entity_key, op, payload, scope, project, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
			ON CONFLICT (source_machine, source_seq) DO NOTHING
		`, m.Seq, cfg.MachineID, m.Entity, m.EntityKey, m.Op, m.Payload, scope, m.Project, occurredAt)
		if err != nil {
			return 0, lastSeq, fmt.Errorf("inserting mutation seq=%d: %w", m.Seq, err)
		}
		pushed++
		if m.Seq > maxSeq {
			maxSeq = m.Seq
		}
	}

	// Update machine's last_push_at.
	_, err = tx.Exec(ctx, `
		UPDATE engram_machines SET last_push_at = NOW() WHERE machine_id = $1
	`, cfg.MachineID)
	if err != nil {
		return 0, lastSeq, fmt.Errorf("updating last_push_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, lastSeq, fmt.Errorf("committing transaction: %w", err)
	}

	if err := WritePushCursor(cursorPath, maxSeq); err != nil {
		logger.Warn("pushed mutations but failed to write cursor", "count", pushed, "error", err)
	}

	return pushed, maxSeq, nil
}

// scopeFromMutation determines the scope from the payload or falls back to config.
func scopeFromMutation(m Mutation, cfg *config.Config) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(m.Payload), &payload); err == nil {
		if s, ok := payload["scope"].(string); ok && s != "" {
			return s
		}
	}
	return cfg.Scope
}
