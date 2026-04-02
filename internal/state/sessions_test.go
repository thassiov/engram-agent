package state

import (
	"testing"
)

func TestGetOrCreateSession_New(t *testing.T) {
	db := testDB(t)

	s, err := db.GetOrCreateSession("sess-1", "/tmp/session.log")
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	if s.SessionID != "sess-1" {
		t.Errorf("SessionID: want %q, got %q", "sess-1", s.SessionID)
	}
	if s.LastTurn != 0 {
		t.Errorf("LastTurn: want 0, got %d", s.LastTurn)
	}
	if s.Status != "active" {
		t.Errorf("Status: want %q, got %q", "active", s.Status)
	}
}

func TestGetOrCreateSession_Existing(t *testing.T) {
	db := testDB(t)

	_, err := db.GetOrCreateSession("sess-2", "/tmp/session.log")
	if err != nil {
		t.Fatalf("first GetOrCreateSession: %v", err)
	}

	// Update last_turn so we can verify the second call returns the same row.
	if err := db.UpdateLastTurn("sess-2", 42); err != nil {
		t.Fatalf("UpdateLastTurn: %v", err)
	}

	s2, err := db.GetOrCreateSession("sess-2", "/tmp/session.log")
	if err != nil {
		t.Fatalf("second GetOrCreateSession: %v", err)
	}
	if s2.LastTurn != 42 {
		t.Errorf("expected existing session with LastTurn=42, got %d", s2.LastTurn)
	}

	// Verify no duplicate row was created.
	var count int
	if err := db.db.QueryRow(`SELECT count(*) FROM session_state WHERE session_id = ?`, "sess-2").Scan(&count); err != nil {
		t.Fatalf("counting sessions: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 session row, got %d", count)
	}
}

func TestUpdateLastTurn(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("sess-3", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	if err := db.UpdateLastTurn("sess-3", 99); err != nil {
		t.Fatalf("UpdateLastTurn: %v", err)
	}

	s, err := db.GetOrCreateSession("sess-3", "/tmp/session.log")
	if err != nil {
		t.Fatalf("re-read session: %v", err)
	}
	if s.LastTurn != 99 {
		t.Errorf("LastTurn: want 99, got %d", s.LastTurn)
	}
}

func TestEndSession(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("sess-4", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	if err := db.EndSession("sess-4"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	var status string
	if err := db.db.QueryRow(`SELECT status FROM session_state WHERE session_id = ?`, "sess-4").Scan(&status); err != nil {
		t.Fatalf("reading status: %v", err)
	}
	if status != "ended" {
		t.Errorf("status: want %q, got %q", "ended", status)
	}
}

func TestResetSession_Empty(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("sess-5", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	obs, vecs, chunks, err := db.ResetSession("sess-5")
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if obs != 0 || vecs != 0 || chunks != 0 {
		t.Errorf("expected 0,0,0 for empty session, got obs=%d vecs=%d chunks=%d", obs, vecs, chunks)
	}
}

func TestResetSession_WithData(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("sess-6", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	// Insert two chunks.
	chunkID1, err := db.SaveChunk("sess-6", 1, 0, 5, 100, "chunk content 1")
	if err != nil {
		t.Fatalf("SaveChunk 1: %v", err)
	}
	chunkID2, err := db.SaveChunk("sess-6", 1, 6, 10, 200, "chunk content 2")
	if err != nil {
		t.Fatalf("SaveChunk 2: %v", err)
	}

	// Insert two observations.
	obsID1, err := db.SaveObservation("sess-6", chunkID1, "decision", "title1", "content1", "project", "general", "key1")
	if err != nil {
		t.Fatalf("SaveObservation 1: %v", err)
	}
	obsID2, err := db.SaveObservation("sess-6", chunkID2, "pattern", "title2", "content2", "project", "general", "key2")
	if err != nil {
		t.Fatalf("SaveObservation 2: %v", err)
	}

	// Insert one vector.
	if err := db.SaveVector(obsID1, []float32{1.0, 2.0, 3.0}); err != nil {
		t.Fatalf("SaveVector: %v", err)
	}
	_ = obsID2 // second observation has no vector

	obs, vecs, chunks, err := db.ResetSession("sess-6")
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if obs != 2 {
		t.Errorf("observations: want 2, got %d", obs)
	}
	if vecs != 1 {
		t.Errorf("vectors: want 1, got %d", vecs)
	}
	if chunks != 2 {
		t.Errorf("chunks: want 2, got %d", chunks)
	}

	// Verify last_turn was reset to 0.
	s, err := db.GetOrCreateSession("sess-6", "/tmp/session.log")
	if err != nil {
		t.Fatalf("re-read session after reset: %v", err)
	}
	if s.LastTurn != 0 {
		t.Errorf("LastTurn after reset: want 0, got %d", s.LastTurn)
	}
}

func TestNextBatchID_First(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("sess-7", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	id, err := db.NextBatchID("sess-7")
	if err != nil {
		t.Fatalf("NextBatchID: %v", err)
	}
	if id != 1 {
		t.Errorf("first NextBatchID: want 1, got %d", id)
	}
}

func TestNextBatchID_Increments(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("sess-8", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	// Insert a chunk with batch_id=3.
	if _, err := db.SaveChunk("sess-8", 3, 0, 5, 100, "content"); err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	id, err := db.NextBatchID("sess-8")
	if err != nil {
		t.Fatalf("NextBatchID: %v", err)
	}
	if id != 4 {
		t.Errorf("NextBatchID after batch_id=3: want 4, got %d", id)
	}
}
