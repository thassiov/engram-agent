package state

import (
	"testing"
)

func TestSaveChunk(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("obs-sess-1", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	id, err := db.SaveChunk("obs-sess-1", 1, 0, 10, 500, "chunk content")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}
	if id <= 0 {
		t.Errorf("SaveChunk: want id > 0, got %d", id)
	}
}

func TestSaveObservation(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("obs-sess-2", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("obs-sess-2", 1, 0, 10, 500, "chunk content")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	id, err := db.SaveObservation("obs-sess-2", chunkID, "decision", "Test title", "Test content", "project", "general", "test-key")
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}
	if id <= 0 {
		t.Errorf("SaveObservation: want id > 0, got %d", id)
	}
}

func TestGetPendingObservations(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("obs-sess-3", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("obs-sess-3", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	// Save two pending observations.
	_, err = db.SaveObservation("obs-sess-3", chunkID, "decision", "Title A", "Content A", "project", "general", "key-a")
	if err != nil {
		t.Fatalf("SaveObservation A: %v", err)
	}
	_, err = db.SaveObservation("obs-sess-3", chunkID, "pattern", "Title B", "Content B", "project", "general", "key-b")
	if err != nil {
		t.Fatalf("SaveObservation B: %v", err)
	}

	pending, err := db.GetPendingObservations("obs-sess-3")
	if err != nil {
		t.Fatalf("GetPendingObservations: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("GetPendingObservations: want 2, got %d", len(pending))
	}
}

func TestGetPendingObservations_IgnoresSavedAndDuplicate(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("obs-sess-4", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("obs-sess-4", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	idPending, err := db.SaveObservation("obs-sess-4", chunkID, "decision", "Pending", "content", "project", "general", "k1")
	if err != nil {
		t.Fatalf("SaveObservation pending: %v", err)
	}

	idSaved, err := db.SaveObservation("obs-sess-4", chunkID, "decision", "Saved", "content", "project", "general", "k2")
	if err != nil {
		t.Fatalf("SaveObservation saved: %v", err)
	}

	idDup, err := db.SaveObservation("obs-sess-4", chunkID, "decision", "Duplicate", "content", "project", "general", "k3")
	if err != nil {
		t.Fatalf("SaveObservation duplicate: %v", err)
	}

	if err := db.MarkObservationSaved(idSaved); err != nil {
		t.Fatalf("MarkObservationSaved: %v", err)
	}
	db.MarkObservationDuplicate(idDup)

	pending, err := db.GetPendingObservations("obs-sess-4")
	if err != nil {
		t.Fatalf("GetPendingObservations: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("want 1 pending, got %d", len(pending))
	}
	if len(pending) > 0 && pending[0].ID != idPending {
		t.Errorf("wrong pending observation: want id=%d, got id=%d", idPending, pending[0].ID)
	}
}

func TestMarkObservationSaved(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("obs-sess-5", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("obs-sess-5", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	id, err := db.SaveObservation("obs-sess-5", chunkID, "decision", "Title", "Content", "project", "general", "key")
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}

	if err := db.MarkObservationSaved(id); err != nil {
		t.Fatalf("MarkObservationSaved: %v", err)
	}

	pending, err := db.GetPendingObservations("obs-sess-5")
	if err != nil {
		t.Fatalf("GetPendingObservations: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after MarkObservationSaved, got %d", len(pending))
	}
}

func TestMarkObservationDuplicate(t *testing.T) {
	db := testDB(t)

	if _, err := db.GetOrCreateSession("obs-sess-6", "/tmp/session.log"); err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	chunkID, err := db.SaveChunk("obs-sess-6", 1, 0, 5, 100, "chunk")
	if err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	id, err := db.SaveObservation("obs-sess-6", chunkID, "decision", "Title", "Content", "project", "general", "key")
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}

	db.MarkObservationDuplicate(id)

	pending, err := db.GetPendingObservations("obs-sess-6")
	if err != nil {
		t.Fatalf("GetPendingObservations: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after MarkObservationDuplicate, got %d", len(pending))
	}
}

func TestLog(t *testing.T) {
	db := testDB(t)
	// Should not panic.
	db.Log("info", "test-component", "log-sess-1", "test log message")
}

func TestRecordEvent(t *testing.T) {
	db := testDB(t)
	// Should not panic.
	db.RecordEvent("event-sess-1", "test-action", "test details")
}
