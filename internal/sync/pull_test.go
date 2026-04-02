package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHttpPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := json.RawMessage(`{"key":"value"}`)
	err := httpPost(context.Background(), srv.URL, payload)
	if err != nil {
		t.Fatalf("expected no error for 200, got: %v", err)
	}
}

func TestHttpPost_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	payload := json.RawMessage(`{"key":"value"}`)
	err := httpPost(context.Background(), srv.URL, payload)
	if err != nil {
		t.Fatalf("expected no error for 409 Conflict, got: %v", err)
	}
}

func TestHttpPost_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	payload := json.RawMessage(`{"key":"value"}`)
	err := httpPost(context.Background(), srv.URL, payload)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestHttpDelete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := httpDelete(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected no error for 200 DELETE, got: %v", err)
	}
}
