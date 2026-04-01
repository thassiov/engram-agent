package state

import (
	"fmt"
)

// SaveChunk records a chunk in the state DB.
func (d *DB) SaveChunk(sessionID string, batchID, turnStart, turnEnd, charCount int, content string) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO chunks (session_id, batch_id, turn_start, turn_end, char_count, content, status)
		 VALUES (?, ?, ?, ?, ?, ?, 'done')`,
		sessionID, batchID, turnStart, turnEnd, charCount, content,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting chunk: %w", err)
	}
	return result.LastInsertId()
}

// SaveObservation records an extracted observation in the state DB.
func (d *DB) SaveObservation(sessionID string, chunkID int64, obsType, title, content, scope, project, topicKey string) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO observations (session_id, chunk_id, type, title, content, scope, project, topic_key, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		sessionID, chunkID, obsType, title, content, scope, project, topicKey,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting observation: %w", err)
	}
	return result.LastInsertId()
}

// MarkObservationSaved marks an observation as saved to engram.
func (d *DB) MarkObservationSaved(id int64) error {
	_, err := d.db.Exec(`UPDATE observations SET status = 'saved' WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("marking observation saved: %w", err)
	}
	return nil
}

// MarkObservationDuplicate marks an observation as a duplicate (won't be saved to engram).
func (d *DB) MarkObservationDuplicate(id int64) {
	d.db.Exec(`UPDATE observations SET status = 'duplicate' WHERE id = ?`, id) //nolint:errcheck
}

// PendingObservation represents an observation ready to be saved to engram.
type PendingObservation struct {
	ID       int64
	Type     string
	Title    string
	Content  string
	Scope    string
	Project  string
	TopicKey string
}

// GetPendingObservations returns observations not yet saved to engram.
func (d *DB) GetPendingObservations(sessionID string) ([]PendingObservation, error) {
	rows, err := d.db.Query(
		`SELECT id, type, title, content, scope, project, topic_key
		 FROM observations
		 WHERE session_id = ? AND status = 'pending'
		 ORDER BY id`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying pending observations: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var obs []PendingObservation
	for rows.Next() {
		var o PendingObservation
		if err := rows.Scan(&o.ID, &o.Type, &o.Title, &o.Content, &o.Scope, &o.Project, &o.TopicKey); err != nil {
			return nil, fmt.Errorf("scanning observation: %w", err)
		}
		obs = append(obs, o)
	}
	return obs, rows.Err()
}

// Log writes a structured log entry to the state DB.
func (d *DB) Log(level, component, sessionID, message string) {
	d.db.Exec( //nolint:errcheck // best-effort logging
		`INSERT INTO logs (level, component, session_id, message) VALUES (?, ?, ?, ?)`,
		level, component, sessionID, message,
	)
}
