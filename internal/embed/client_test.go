package embed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"embeddings":[[0.1, 0.2, 0.3], [0.4, 0.5, 0.6]]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := New(srv.URL)
	vectors, err := client.Embed(context.Background(), []string{"text1", "text2"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if len(vectors[0]) != 3 {
		t.Errorf("expected first vector to have 3 elements, got %d", len(vectors[0]))
	}
	if vectors[0][0] != 0.1 {
		t.Errorf("expected vectors[0][0] = 0.1, got %v", vectors[0][0])
	}
	if vectors[1][2] != 0.6 {
		t.Errorf("expected vectors[1][2] = 0.6, got %v", vectors[1][2])
	}
}

func TestEmbed_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(srv.URL)
	_, err := client.Embed(context.Background(), []string{"text"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestEmbedOne_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"embeddings":[[0.1, 0.2, 0.3]]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := New(srv.URL)
	vec, err := client.EmbedOne(context.Background(), "hello")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(vec) != 3 {
		t.Errorf("expected vector of length 3, got %d", len(vec))
	}
	if vec[0] != 0.1 {
		t.Errorf("expected vec[0] = 0.1, got %v", vec[0])
	}
}

func TestEmbedOne_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"embeddings":[]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := New(srv.URL)
	_, err := client.EmbedOne(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for empty embeddings response, got nil")
	}
}

func TestReachable_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := New(srv.URL)
	if !client.Reachable(context.Background()) {
		t.Fatal("expected Reachable to return true")
	}
}

func TestReachable_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := New(srv.URL)
	if client.Reachable(context.Background()) {
		t.Fatal("expected Reachable to return false for 503")
	}
}
