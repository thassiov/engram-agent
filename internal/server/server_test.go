package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(handler NotifyHandler) *Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(":0", handler, logger)
}

func TestHandleNotify_ValidPayload(t *testing.T) {
	var received Notification
	notified := make(chan struct{}, 1)

	srv := newTestServer(func(n Notification) {
		received = n
		notified <- struct{}{}
	})

	body := bytes.NewBufferString(`{"session_id":"sess-1","event":"stop"}`)
	req := httptest.NewRequest(http.MethodPost, "/notify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleNotify(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}

	// Wait for async handler call.
	<-notified

	if received.SessionID != "sess-1" {
		t.Errorf("expected SessionID='sess-1', got %q", received.SessionID)
	}
	if received.Event != "stop" {
		t.Errorf("expected Event='stop', got %q", received.Event)
	}
}

func TestHandleNotify_MissingSessionID(t *testing.T) {
	srv := newTestServer(func(n Notification) {
		t.Error("handler should not be called for missing session_id")
	})

	body := bytes.NewBufferString(`{"event":"stop"}`)
	req := httptest.NewRequest(http.MethodPost, "/notify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleNotify(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleNotify_InvalidJSON(t *testing.T) {
	srv := newTestServer(func(n Notification) {
		t.Error("handler should not be called for invalid JSON")
	})

	body := bytes.NewBufferString(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/notify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleNotify(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(func(n Notification) {})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	body := string(bodyBytes)
	if body != `{"status":"ok"}` {
		t.Errorf("expected body={\"status\":\"ok\"}, got %q", body)
	}
}
