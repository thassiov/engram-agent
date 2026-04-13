package sync

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// VectorPushCursorFile returns the path to the vector push cursor file.
func VectorPushCursorFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "engram", "vector-push-cursor")
}

// ReadVectorCursor reads the last pushed engram_obs_id.
func ReadVectorCursor(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// WriteVectorCursor writes the vector push cursor.
func WriteVectorCursor(path string, id int64) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.FormatInt(id, 10)), 0o644)
}

// PushVectors syncs engram_vectors from state.db to PG's engram_vectors table.
// Returns the number of vectors pushed.
func PushVectors(ctx context.Context, stateDB *sql.DB, pgConn *pgx.Conn, machineID string, logger *slog.Logger) (int, error) {
	cursorPath := VectorPushCursorFile()
	lastID, err := ReadVectorCursor(cursorPath)
	if err != nil {
		return 0, fmt.Errorf("reading vector cursor: %w", err)
	}

	// Read new vectors from state.db.
	rows, err := stateDB.Query(`
		SELECT engram_obs_id, title, COALESCE(content, ''), type, scope, project,
		       COALESCE(topic_key, ''), embedding, dims
		FROM engram_vectors
		WHERE engram_obs_id > ?
		ORDER BY engram_obs_id ASC
	`, lastID)
	if err != nil {
		return 0, fmt.Errorf("querying engram_vectors: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	type vectorRow struct {
		engramObsID                                         int64
		title, content, obsType, scope, project, topicKey string
		embedding                                         []byte
		dims                                              int
	}

	var pending []vectorRow
	for rows.Next() {
		var v vectorRow
		if err := rows.Scan(&v.engramObsID, &v.title, &v.content, &v.obsType, &v.scope, &v.project, &v.topicKey, &v.embedding, &v.dims); err != nil {
			return 0, fmt.Errorf("scanning vector row: %w", err)
		}
		pending = append(pending, v)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterating vector rows: %w", err)
	}

	if len(pending) == 0 {
		return 0, nil
	}

	// Push to PG in a single transaction.
	tx, err := pgConn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning vector tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var maxID int64
	pushed := 0
	for _, v := range pending {
		// Convert binary blob to pgvector string format: [0.1,0.2,...]
		vecStr := blobToVectorString(v.embedding, v.dims)

		_, err := tx.Exec(ctx, `
			INSERT INTO engram_vectors (source_machine, engram_obs_id, title, content, type, scope, project, topic_key, embedding)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector)
			ON CONFLICT (source_machine, engram_obs_id) DO NOTHING
		`, machineID, v.engramObsID, v.title, v.content, v.obsType, v.scope, v.project, v.topicKey, vecStr)
		if err != nil {
			return 0, fmt.Errorf("inserting vector engram_obs_id=%d: %w", v.engramObsID, err)
		}
		pushed++
		if v.engramObsID > maxID {
			maxID = v.engramObsID
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing vector tx: %w", err)
	}

	if err := WriteVectorCursor(cursorPath, maxID); err != nil {
		logger.Warn("pushed vectors but failed to write cursor", "count", pushed, "error", err)
	}

	return pushed, nil
}

// blobToVectorString converts a binary float32 blob to pgvector text format: [0.1,0.2,...].
func blobToVectorString(blob []byte, dims int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := range dims {
		if i > 0 {
			b.WriteByte(',')
		}
		bits := binary.LittleEndian.Uint32(blob[i*4:])
		f := math.Float32frombits(bits)
		b.WriteString(strconv.FormatFloat(float64(f), 'f', 6, 32))
	}
	b.WriteByte(']')
	return b.String()
}
