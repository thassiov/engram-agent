package state

import (
	"math"
	"testing"
)

// DB tests for vectors.

func TestSaveVector(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("vec-sess-1", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("vec-sess-1", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	obsID, err := db.SaveObservation("vec-sess-1", chunkID, "decision", "Title", "Content", "project", "general", "key")
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}

	vec := []float32{0.1, 0.2, 0.3}
	if err := db.SaveVector(obsID, vec); err != nil {
		t.Fatalf("SaveVector: %v", err)
	}

	entries, err := db.AllVectors()
	if err != nil {
		t.Fatalf("AllVectors: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("AllVectors: want 1, got %d", len(entries))
	}
	if entries[0].ObservationID != obsID {
		t.Errorf("ObservationID: want %d, got %d", obsID, entries[0].ObservationID)
	}
	if len(entries[0].Vector) != len(vec) {
		t.Fatalf("Vector length: want %d, got %d", len(vec), len(entries[0].Vector))
	}
	for i := range vec {
		if entries[0].Vector[i] != vec[i] {
			t.Errorf("Vector[%d]: want %f, got %f", i, vec[i], entries[0].Vector[i])
		}
	}
}

func TestAllVectors_Empty(t *testing.T) {
	db := testDB(t)

	entries, err := db.AllVectors()
	if err != nil {
		t.Fatalf("AllVectors: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("AllVectors on empty DB: want 0, got %d", len(entries))
	}
}

func TestAllVectors_Multiple(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("vec-sess-2", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("vec-sess-2", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	for i := 0; i < 3; i++ {
		obsID, err := db.SaveObservation("vec-sess-2", chunkID, "decision", "Title", "Content", "project", "general", "key")
		if err != nil {
			t.Fatalf("SaveObservation %d: %v", i, err)
		}
		if err := db.SaveVector(obsID, []float32{float32(i), float32(i + 1)}); err != nil {
			t.Fatalf("SaveVector %d: %v", i, err)
		}
	}

	entries, err := db.AllVectors()
	if err != nil {
		t.Fatalf("AllVectors: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("AllVectors: want 3, got %d", len(entries))
	}
}

func TestSaveVector_Upsert(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("vec-sess-3", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("vec-sess-3", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	obsID, err := db.SaveObservation("vec-sess-3", chunkID, "decision", "Title", "Content", "project", "general", "key")
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}

	// Save initial vector.
	if err := db.SaveVector(obsID, []float32{1.0, 2.0, 3.0}); err != nil {
		t.Fatalf("SaveVector (first): %v", err)
	}

	// Save updated vector for same observation_id.
	updated := []float32{9.0, 8.0, 7.0}
	if err := db.SaveVector(obsID, updated); err != nil {
		t.Fatalf("SaveVector (upsert): %v", err)
	}

	entries, err := db.AllVectors()
	if err != nil {
		t.Fatalf("AllVectors: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("AllVectors after upsert: want 1, got %d", len(entries))
	}
	for i, v := range updated {
		if entries[0].Vector[i] != v {
			t.Errorf("upserted Vector[%d]: want %f, got %f", i, v, entries[0].Vector[i])
		}
	}
}

func TestEncodeDecodeVector_Roundtrip(t *testing.T) {
	original := []float32{1.5, -2.75, 0.0, 3.14159, math.MaxFloat32}
	encoded := encodeVector(original)
	decoded := decodeVector(encoded, len(original))

	if len(decoded) != len(original) {
		t.Fatalf("decoded length %d != original length %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("mismatch at index %d: want %f, got %f", i, original[i], decoded[i])
		}
	}
}

func TestEncodeDecodeVector_Empty(t *testing.T) {
	var empty []float32
	encoded := encodeVector(empty)
	if len(encoded) != 0 {
		t.Errorf("expected empty byte slice for empty vector, got %d bytes", len(encoded))
	}
	decoded := decodeVector(encoded, 0)
	if len(decoded) != 0 {
		t.Errorf("expected empty float slice for empty bytes, got %d elements", len(decoded))
	}
}
