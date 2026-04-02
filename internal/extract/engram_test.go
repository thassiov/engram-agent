package extract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSaveToEngram_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/observations" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	obs := Observation{
		Type:    "decision",
		Title:   "Test decision",
		Content: "We decided something.",
		Scope:   "personal",
		Project: "general",
	}
	err := SaveToEngram(context.Background(), srv.URL, "session-123", obs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSaveToEngram_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	obs := Observation{
		Type:    "decision",
		Title:   "Test decision",
		Content: "We decided something.",
	}
	err := SaveToEngram(context.Background(), srv.URL, "session-abc", obs)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestSaveToEngram_RequestBody(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		capturedBody = body
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	obs := Observation{
		Type:    "bugfix",
		Title:   "Fixed the thing",
		Content: "Root cause was X, fix was Y.",
		Scope:   "personal",
		Project: "engram",
	}
	const sessionID = "session-xyz"
	err := SaveToEngram(context.Background(), srv.URL, sessionID, obs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("could not parse request body as JSON: %v", err)
	}

	if v, ok := payload["session_id"]; !ok || v != sessionID {
		t.Errorf("expected session_id=%q in body, got %v", sessionID, payload["session_id"])
	}
	if v, ok := payload["type"]; !ok || v != obs.Type {
		t.Errorf("expected type=%q in body, got %v", obs.Type, payload["type"])
	}
	if v, ok := payload["title"]; !ok || v != obs.Title {
		t.Errorf("expected title=%q in body, got %v", obs.Title, payload["title"])
	}
	if v, ok := payload["content"]; !ok || v != obs.Content {
		t.Errorf("expected content=%q in body, got %v", obs.Content, payload["content"])
	}
	if v, ok := payload["tool_name"]; !ok || v != "engram-agent" {
		t.Errorf("expected tool_name=%q in body, got %v", "engram-agent", payload["tool_name"])
	}
}
