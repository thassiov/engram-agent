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
