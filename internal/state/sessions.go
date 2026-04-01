package state

import (
	"fmt"
)

// Session represents tracked session state.
type Session struct {
	SessionID  string
	SessionLog string
	LastTurn   int
	Status     string
}

// GetOrCreateSession returns the session state, creating it if it doesn't exist.
func (d *DB) GetOrCreateSession(sessionID, sessionLog string) (*Session, error) {
	var s Session
	err := d.db.QueryRow(
		`SELECT session_id, session_log, last_turn, status FROM session_state WHERE session_id = ?`,
		sessionID,
	).Scan(&s.SessionID, &s.SessionLog, &s.LastTurn, &s.Status)

	if err == nil {
		return &s, nil
	}

	// Create new session.
	_, err = d.db.Exec(
		`INSERT INTO session_state (session_id, session_log, last_turn, status) VALUES (?, ?, 0, 'active')`,
		sessionID, sessionLog,
	)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return &Session{
		SessionID:  sessionID,
		SessionLog: sessionLog,
		LastTurn:   0,
		Status:     "active",
	}, nil
}

// UpdateLastTurn updates the last extracted turn count.
func (d *DB) UpdateLastTurn(sessionID string, lastTurn int) error {
	_, err := d.db.Exec(
		`UPDATE session_state SET last_turn = ?, updated_at = datetime('now') WHERE session_id = ?`,
		lastTurn, sessionID,
	)
	if err != nil {
		return fmt.Errorf("updating last_turn: %w", err)
	}
	return nil
}

// EndSession marks a session as ended.
func (d *DB) EndSession(sessionID string) error {
	_, err := d.db.Exec(
		`UPDATE session_state SET status = 'ended', updated_at = datetime('now') WHERE session_id = ?`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("ending session: %w", err)
	}
	return nil
}

// ResetSession deletes all data for a session (observations, vectors, chunks)
// and resets last_turn to 0. Returns counts of what was deleted.
func (d *DB) ResetSession(sessionID string) (observations, vectors, chunks int, err error) {
	// Count before deleting.
	d.db.QueryRow(`SELECT count(*) FROM observations WHERE session_id = ?`, sessionID).Scan(&observations)
	d.db.QueryRow(`SELECT count(*) FROM vectors WHERE observation_id IN (SELECT id FROM observations WHERE session_id = ?)`, sessionID).Scan(&vectors)
	d.db.QueryRow(`SELECT count(*) FROM chunks WHERE session_id = ?`, sessionID).Scan(&chunks)

	// Delete vectors for this session's observations.
	_, err = d.db.Exec(`DELETE FROM vectors WHERE observation_id IN (SELECT id FROM observations WHERE session_id = ?)`, sessionID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("deleting vectors: %w", err)
	}

	// Delete observations.
	_, err = d.db.Exec(`DELETE FROM observations WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("deleting observations: %w", err)
	}

	// Delete chunks.
	_, err = d.db.Exec(`DELETE FROM chunks WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("deleting chunks: %w", err)
	}

	// Reset turn counter.
	_, err = d.db.Exec(`UPDATE session_state SET last_turn = 0, updated_at = datetime('now') WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("resetting last_turn: %w", err)
	}

	return observations, vectors, chunks, nil
}

// NextBatchID returns the next batch ID for a session.
func (d *DB) NextBatchID(sessionID string) (int, error) {
	var maxBatch int
	err := d.db.QueryRow(
		`SELECT COALESCE(MAX(batch_id), 0) FROM chunks WHERE session_id = ?`,
		sessionID,
	).Scan(&maxBatch)
	if err != nil {
		return 0, fmt.Errorf("reading max batch_id: %w", err)
	}
	return maxBatch + 1, nil
}
