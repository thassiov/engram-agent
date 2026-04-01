// Package server provides the HTTP listener for Claude Code hook notifications.
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// Notification represents a hook event from Claude Code.
type Notification struct {
	SessionID string `json:"session_id"`
	Event     string `json:"event,omitempty"` // "stop", "force", or empty for prompt submit
	Reset     bool   `json:"reset,omitempty"` // Reset last_turn to 0 before extracting.
}

// NotifyHandler is called when a hook notification is received.
type NotifyHandler func(n Notification)

// Server listens for hook notifications from Claude Code.
type Server struct {
	addr    string
	handler NotifyHandler
	logger  *slog.Logger
}

// New creates a new notification server.
func New(addr string, handler NotifyHandler, logger *slog.Logger) *Server {
	return &Server{
		addr:    addr,
		handler: handler,
		logger:  logger,
	}
}

// ListenAndServe starts the HTTP server. Blocks until the server is shut down.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /notify", s.handleNotify)
	mux.HandleFunc("GET /health", s.handleHealth)

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	s.logger.Info("hook listener started", "addr", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	var n Notification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		s.logger.Warn("invalid notification payload", "error", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if n.SessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}

	s.logger.Debug("notification received", "session_id", n.SessionID, "event", n.Event)

	// Handle asynchronously so the hook returns fast.
	go s.handler(n)

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck // best-effort
}
