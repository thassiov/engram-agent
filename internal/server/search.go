// Package server provides the HTTP listener for Claude Code hook notifications.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/thassiov/engram-agent/internal/embed"
	"github.com/thassiov/engram-agent/internal/state"
)

// SearchResult is a single observation returned by semantic search.
type SearchResult struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Project    string  `json:"project"`
	TopicKey   string  `json:"topic_key,omitempty"`
	Similarity float64 `json:"similarity"`
}

// SearchHandler handles semantic vector search requests.
type SearchHandler struct {
	stateDB     *state.DB
	embedClient *embed.Client
	logger      *slog.Logger

	mu      sync.RWMutex
	cache   []state.SearchableVector
	loadedAt time.Time
}

const cacheTTL = 5 * time.Minute

// NewSearchHandler creates a new search handler.
func NewSearchHandler(stateDB *state.DB, embedClient *embed.Client, logger *slog.Logger) *SearchHandler {
	return &SearchHandler{
		stateDB:     stateDB,
		embedClient: embedClient,
		logger:      logger,
	}
}

func (h *SearchHandler) getCache() ([]state.SearchableVector, error) {
	h.mu.RLock()
	if time.Since(h.loadedAt) < cacheTTL && h.cache != nil {
		defer h.mu.RUnlock()
		return h.cache, nil
	}
	h.mu.RUnlock()

	if err := h.RefreshCache(); err != nil {
		return nil, err
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cache, nil
}

// RefreshCache reloads vectors from both state.db native observations and engram.db backfill vectors.
func (h *SearchHandler) RefreshCache() error {
	native, err := h.stateDB.SearchableVectors()
	if err != nil {
		return err
	}
	engram, err := h.stateDB.SearchableEngramVectors()
	if err != nil {
		return err
	}

	combined := make([]state.SearchableVector, 0, len(native)+len(engram))
	combined = append(combined, native...)
	combined = append(combined, engram...)

	h.mu.Lock()
	h.cache = combined
	h.loadedAt = time.Now()
	h.mu.Unlock()
	h.logger.Info("vector cache refreshed", "native", len(native), "engram", len(engram), "total", len(combined))
	return nil
}

// Handle serves GET /search?q=<query>&limit=<N>&project=<project>.
func (h *SearchHandler) Handle(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `{"error":"q parameter required"}`, http.StatusBadRequest)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}

	projectFilter := r.URL.Query().Get("project")

	// Check fastembed availability and embed the query.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if !h.embedClient.Reachable(ctx) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"error":    "embedding service unavailable",
			"fallback": "use mem_search for keyword search",
		})
		return
	}

	queryVec, err := h.embedClient.EmbedOne(ctx, query)
	if err != nil {
		h.logger.Error("failed to embed query", "error", err)
		http.Error(w, `{"error":"failed to embed query"}`, http.StatusInternalServerError)
		return
	}

	// Load cached vectors.
	entries, err := h.getCache()
	if err != nil {
		h.logger.Error("failed to load vectors", "error", err)
		http.Error(w, `{"error":"failed to load vectors"}`, http.StatusInternalServerError)
		return
	}

	// Compute similarities.
	type scored struct {
		entry      state.SearchableVector
		similarity float64
	}
	var results []scored
	for _, e := range entries {
		if projectFilter != "" && e.Project != projectFilter {
			continue
		}
		sim := embed.CosineSimilarity(queryVec, e.Vector)
		results = append(results, scored{entry: e, similarity: sim})
	}

	// Sort descending by similarity.
	sort.Slice(results, func(i, j int) bool {
		return results[i].similarity > results[j].similarity
	})

	// Take top N.
	if len(results) > limit {
		results = results[:limit]
	}

	// Build response.
	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = SearchResult{
			ID:         r.entry.ObservationID,
			Type:       r.entry.Type,
			Title:      r.entry.Title,
			Content:    r.entry.Content,
			Project:    r.entry.Project,
			TopicKey:   r.entry.TopicKey,
			Similarity: r.similarity,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out) //nolint:errcheck
}
