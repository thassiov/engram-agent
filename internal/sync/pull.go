package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/thassiov/engram-agent/internal/config"
)

// RemoteMutation represents a row from the central PG engram_sync_mutations table.
type RemoteMutation struct {
	ID            int64
	SourceSeq     int64
	SourceMachine string
	Entity        string
	EntityKey     string
	Op            string
	Payload       json.RawMessage
	Scope         string
	Project       string
	OccurredAt    time.Time
}

// ReadCursor reads this machine's pull cursor from PostgreSQL.
func ReadCursor(ctx context.Context, pgConn *pgx.Conn, machineID string) (int64, error) {
	var lastSeq int64
	err := pgConn.QueryRow(ctx,
		`SELECT last_seq FROM engram_sync_cursors WHERE consumer_machine = $1`,
		machineID,
	).Scan(&lastSeq)
	if err != nil {
		return 0, fmt.Errorf("reading cursor for %s: %w", machineID, err)
	}
	return lastSeq, nil
}

// UpdateCursor updates this machine's pull cursor in PostgreSQL.
func UpdateCursor(ctx context.Context, pgConn *pgx.Conn, machineID string, seq int64) error {
	_, err := pgConn.Exec(ctx, `
		UPDATE engram_sync_cursors
		SET last_seq = $1, last_synced_at = NOW()
		WHERE consumer_machine = $2
	`, seq, machineID)
	if err != nil {
		return fmt.Errorf("updating cursor: %w", err)
	}
	_, err = pgConn.Exec(ctx, `
		UPDATE engram_machines SET last_pull_at = NOW() WHERE machine_id = $1
	`, machineID)
	if err != nil {
		return fmt.Errorf("updating last_pull_at: %w", err)
	}
	return nil
}

// FetchMutations fetches new mutations from PG for this machine, applying scope filters.
func FetchMutations(ctx context.Context, pgConn *pgx.Conn, cfg *config.Config, afterSeq int64) ([]RemoteMutation, error) {
	query := `
		SELECT id, source_seq, source_machine, entity, entity_key, op, payload, scope, project, occurred_at
		FROM engram_sync_mutations
		WHERE id > $1 AND source_machine != $2
	`
	args := []interface{}{afterSeq, cfg.MachineID}

	filterTypes := cfg.PullFilterTypes()
	if filterTypes != nil {
		placeholders := make([]string, len(filterTypes))
		for i, t := range filterTypes {
			args = append(args, t)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		query += fmt.Sprintf(` AND (
			entity != 'observation'
			OR payload->>'type' IN (%s)
		)`, strings.Join(placeholders, ", "))
	}

	query += ` ORDER BY id ASC LIMIT 1000`

	rows, err := pgConn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetching mutations: %w", err)
	}
	defer rows.Close()

	var mutations []RemoteMutation
	for rows.Next() {
		var m RemoteMutation
		if err := rows.Scan(&m.ID, &m.SourceSeq, &m.SourceMachine, &m.Entity, &m.EntityKey, &m.Op, &m.Payload, &m.Scope, &m.Project, &m.OccurredAt); err != nil {
			return nil, fmt.Errorf("scanning mutation: %w", err)
		}
		mutations = append(mutations, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating mutations: %w", err)
	}
	return mutations, nil
}

// ApplyMutation applies a single mutation to the local engram instance via its HTTP API.
func ApplyMutation(ctx context.Context, engramAPI string, m RemoteMutation) error {
	switch m.Entity {
	case "observation":
		return applyObservation(ctx, engramAPI, m)
	case "session":
		return applySession(ctx, engramAPI, m)
	case "prompt":
		return applyPrompt(ctx, engramAPI, m)
	default:
		return nil // unknown entity, skip
	}
}

func applyObservation(ctx context.Context, baseURL string, m RemoteMutation) error {
	if m.Op == "delete" {
		id, err := findObservationBySyncID(ctx, baseURL, m.EntityKey)
		if err != nil {
			return fmt.Errorf("finding observation by sync_id %s: %w", m.EntityKey, err)
		}
		if id == 0 {
			return nil // not found locally
		}
		return httpDelete(ctx, fmt.Sprintf("%s/observations/%d", baseURL, id))
	}
	return httpPost(ctx, baseURL+"/observations", m.Payload)
}

func applySession(ctx context.Context, baseURL string, m RemoteMutation) error {
	if m.Op == "delete" {
		return nil
	}
	return httpPost(ctx, baseURL+"/sessions", m.Payload)
}

func applyPrompt(ctx context.Context, baseURL string, m RemoteMutation) error {
	if m.Op == "delete" {
		return nil
	}
	return httpPost(ctx, baseURL+"/prompts", m.Payload)
}

func findObservationBySyncID(ctx context.Context, baseURL, syncID string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/search?q="+syncID+"&limit=1", http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("creating search request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("searching for observation: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != http.StatusOK {
		return 0, nil
	}

	var result []struct {
		ID     int64  `json:"id"`
		SyncID string `json:"sync_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding search response: %w", err)
	}
	for _, r := range result {
		if r.SyncID == syncID {
			return r.ID, nil
		}
	}
	return 0, nil
}

func httpPost(ctx context.Context, url string, body json.RawMessage) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request to %s: %w", url, err)
	}
	defer resp.Body.Close()               //nolint:errcheck // best-effort cleanup
	_, _ = io.Copy(io.Discard, resp.Body) // drain body for connection reuse

	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
}

func httpDelete(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("creating delete request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending delete to %s: %w", url, err)
	}
	defer resp.Body.Close()               //nolint:errcheck // best-effort cleanup
	_, _ = io.Copy(io.Discard, resp.Body) // drain body for connection reuse
	return nil
}

// Pull fetches new mutations from PG in batches and applies them to the local
// engram instance. Returns the total number of mutations applied and the final cursor.
func Pull(ctx context.Context, pgConn *pgx.Conn, cfg *config.Config, logger *slog.Logger) (applied int, cursor int64, err error) {
	cursor, err = ReadCursor(ctx, pgConn, cfg.MachineID)
	if err != nil {
		return 0, 0, err
	}

	for {
		mutations, fetchErr := FetchMutations(ctx, pgConn, cfg, cursor)
		if fetchErr != nil {
			return applied, cursor, fetchErr
		}
		if len(mutations) == 0 {
			break
		}

		var batchMax int64
		for i := range mutations {
			m := &mutations[i]
			if applyErr := ApplyMutation(ctx, cfg.EngramAPI, *m); applyErr != nil {
				logger.Warn("failed to apply mutation", "id", m.ID, "entity", m.Entity, "error", applyErr)
				continue
			}
			applied++
			if m.ID > batchMax {
				batchMax = m.ID
			}
		}

		if batchMax > 0 {
			if updateErr := UpdateCursor(ctx, pgConn, cfg.MachineID, batchMax); updateErr != nil {
				logger.Warn("applied mutations but failed to update cursor", "count", applied, "error", updateErr)
			}
			cursor = batchMax
		}
	}

	return applied, cursor, nil
}
