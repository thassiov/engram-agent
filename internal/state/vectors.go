package state

import (
	"encoding/binary"
	"fmt"
	"math"
)

// SaveVector stores an embedding vector for an observation.
func (d *DB) SaveVector(observationID int64, vector []float32) error {
	blob := encodeVector(vector)
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO vectors (observation_id, embedding, dims) VALUES (?, ?, ?)`,
		observationID, blob, len(vector),
	)
	if err != nil {
		return fmt.Errorf("saving vector: %w", err)
	}
	return nil
}

// VectorEntry holds a stored vector with its observation ID.
type VectorEntry struct {
	ObservationID int64
	Vector        []float32
}

// AllVectors loads all stored vectors. At small scale this is fine for brute-force dedup.
func (d *DB) AllVectors() ([]VectorEntry, error) {
	rows, err := d.db.Query(`SELECT observation_id, embedding, dims FROM vectors`)
	if err != nil {
		return nil, fmt.Errorf("querying vectors: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []VectorEntry
	for rows.Next() {
		var (
			id   int64
			blob []byte
			dims int
		)
		if err := rows.Scan(&id, &blob, &dims); err != nil {
			return nil, fmt.Errorf("scanning vector: %w", err)
		}
		vec := decodeVector(blob, dims)
		entries = append(entries, VectorEntry{ObservationID: id, Vector: vec})
	}
	return entries, rows.Err()
}

// SearchableVector holds a stored vector with its observation metadata.
type SearchableVector struct {
	ObservationID int64
	Title         string
	Content       string
	Type          string
	Scope         string
	Project       string
	TopicKey      string
	Vector        []float32
}

// SearchableVectors loads all saved vectors joined with their observation content.
func (d *DB) SearchableVectors() ([]SearchableVector, error) {
	rows, err := d.db.Query(`
		SELECT v.observation_id, o.title, o.content, o.type, o.scope, o.project,
		       COALESCE(o.topic_key, ''), v.embedding, v.dims
		FROM vectors v
		JOIN observations o ON o.id = v.observation_id
		WHERE o.status = 'saved'
	`)
	if err != nil {
		return nil, fmt.Errorf("querying searchable vectors: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []SearchableVector
	for rows.Next() {
		var (
			e    SearchableVector
			blob []byte
			dims int
		)
		if err := rows.Scan(&e.ObservationID, &e.Title, &e.Content, &e.Type, &e.Scope, &e.Project, &e.TopicKey, &blob, &dims); err != nil {
			return nil, fmt.Errorf("scanning searchable vector: %w", err)
		}
		e.Vector = decodeVector(blob, dims)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// MissingVectorObs holds an observation that needs vector backfill.
type MissingVectorObs struct {
	ID      int64
	Title   string
	Content string
}

// ObservationsMissingVectors returns saved observations without a vector.
func (d *DB) ObservationsMissingVectors() ([]MissingVectorObs, error) {
	rows, err := d.db.Query(`
		SELECT o.id, o.title, COALESCE(o.content, '')
		FROM observations o
		LEFT JOIN vectors v ON o.id = v.observation_id
		WHERE o.status = 'saved' AND v.observation_id IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("querying missing vectors: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var obs []MissingVectorObs
	for rows.Next() {
		var o MissingVectorObs
		if err := rows.Scan(&o.ID, &o.Title, &o.Content); err != nil {
			return nil, fmt.Errorf("scanning observation: %w", err)
		}
		obs = append(obs, o)
	}
	return obs, rows.Err()
}

// SaveEngramVector stores an embedding for an engram.db observation (by engram.db ID).
func (d *DB) SaveEngramVector(engramObsID int64, title, content, obsType, scope, project, topicKey string, vector []float32) error {
	blob := encodeVector(vector)
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO engram_vectors (engram_obs_id, title, content, type, scope, project, topic_key, embedding, dims)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		engramObsID, title, content, obsType, scope, project, topicKey, blob, len(vector),
	)
	if err != nil {
		return fmt.Errorf("saving engram vector: %w", err)
	}
	return nil
}

// SearchableEngramVectors loads all engram.db vectors for search.
func (d *DB) SearchableEngramVectors() ([]SearchableVector, error) {
	rows, err := d.db.Query(`
		SELECT engram_obs_id, title, COALESCE(content, ''), type, scope, project,
		       COALESCE(topic_key, ''), embedding, dims
		FROM engram_vectors
	`)
	if err != nil {
		return nil, fmt.Errorf("querying engram vectors: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []SearchableVector
	for rows.Next() {
		var (
			e    SearchableVector
			blob []byte
			dims int
		)
		if err := rows.Scan(&e.ObservationID, &e.Title, &e.Content, &e.Type, &e.Scope, &e.Project, &e.TopicKey, &blob, &dims); err != nil {
			return nil, fmt.Errorf("scanning engram vector: %w", err)
		}
		e.Vector = decodeVector(blob, dims)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// EngramVectorIDs returns the set of engram.db observation IDs that already have vectors.
func (d *DB) EngramVectorIDs() (map[int64]bool, error) {
	rows, err := d.db.Query(`SELECT engram_obs_id FROM engram_vectors`)
	if err != nil {
		return nil, fmt.Errorf("querying engram vector IDs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	ids := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning engram vector ID: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// encodeVector converts float32 slice to bytes (little-endian).
func encodeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector converts bytes back to float32 slice.
func decodeVector(buf []byte, dims int) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return v
}
