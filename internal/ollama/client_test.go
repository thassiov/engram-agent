package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":{"role":"assistant","content":"response text"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := New(srv.URL, "test-model")
	content, err := client.Chat(context.Background(), "system prompt", "user message")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if content != "response text" {
		t.Errorf("expected 'response text', got: %q", content)
	}
}

func TestChat_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(srv.URL, "test-model")
	_, err := client.Chat(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestChat_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json at all`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := New(srv.URL, "test-model")
	_, err := client.Chat(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestChat_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":{"role":"assistant","content":"hi"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	client := New(srv.URL, "test-model")
	_, err := client.Chat(ctx, "system", "user")
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

func TestReachable_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := New(srv.URL, "test-model")
	if !client.Reachable(context.Background()) {
		t.Fatal("expected Reachable to return true")
	}
}

func TestReachable_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := New(srv.URL, "test-model")
	if client.Reachable(context.Background()) {
		t.Fatal("expected Reachable to return false for 503")
	}
}

func TestReachable_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close immediately so the server is unreachable

	client := New(srv.URL, "test-model")
	if client.Reachable(context.Background()) {
		t.Fatal("expected Reachable to return false for closed server")
	}
}
