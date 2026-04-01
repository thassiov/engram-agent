package extract

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thassiov/engram-agent/internal/ollama"
	"github.com/thassiov/engram-agent/internal/state"
)

const (
	// TriggerEvery is the number of new turns before triggering extraction.
	TriggerEvery = 15
)

// Watcher tracks sessions and triggers extraction when enough turns accumulate.
type Watcher struct {
	stateDB    *state.DB
	ollamaURL  string
	ollamaModel string
	engramAPI  string
	logger     *slog.Logger
	mu         sync.Mutex // guards concurrent notifications for the same session
}

// NewWatcher creates a new session watcher.
func NewWatcher(stateDB *state.DB, ollamaURL, ollamaModel, engramAPI string, logger *slog.Logger) *Watcher {
	return &Watcher{
		stateDB:     stateDB,
		ollamaURL:   ollamaURL,
		ollamaModel: ollamaModel,
		engramAPI:   engramAPI,
		logger:      logger,
	}
}

// HandleNotification processes a hook notification from Claude Code.
func (w *Watcher) HandleNotification(ctx context.Context, sessionID, event string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Find the session log file.
	sessionLog := findSessionLog(sessionID)
	if sessionLog == "" {
		w.logger.Warn("session log not found", "session_id", sessionID)
		return
	}

	// Get or create session state.
	session, err := w.stateDB.GetOrCreateSession(sessionID, sessionLog)
	if err != nil {
		w.logger.Error("failed to get session state", "session_id", sessionID, "error", err)
		return
	}

	// Count current turns.
	currentTurns, err := countUserTurns(sessionLog)
	if err != nil {
		w.logger.Error("failed to count turns", "session_id", sessionID, "error", err)
		return
	}

	newTurns := currentTurns - session.LastTurn

	if event == "stop" {
		// Session ending — extract any remaining turns.
		if newTurns > 0 {
			w.logger.Info("session ending, extracting remaining turns",
				"session_id", sessionID,
				"new_turns", newTurns,
			)
			w.runExtraction(ctx, session, currentTurns)
		}
		if err := w.stateDB.EndSession(sessionID); err != nil {
			w.logger.Error("failed to end session", "session_id", sessionID, "error", err)
		}
		return
	}

	// Regular notification — check if we have enough turns.
	if newTurns < TriggerEvery {
		return
	}

	w.logger.Info("extraction triggered",
		"session_id", sessionID,
		"new_turns", newTurns,
		"total_turns", currentTurns,
	)

	w.runExtraction(ctx, session, currentTurns)
}

func (w *Watcher) runExtraction(ctx context.Context, session *state.Session, totalTurns int) {
	// Check ollama reachability.
	client := ollama.New(w.ollamaURL, w.ollamaModel)
	if !client.Reachable(ctx) {
		w.logger.Warn("ollama unreachable, queuing for later",
			"session_id", session.SessionID,
			"url", w.ollamaURL,
		)
		w.stateDB.Log("warn", "watcher", session.SessionID,
			fmt.Sprintf("ollama unreachable at %s, extraction skipped", w.ollamaURL))
		return
	}

	// Parse turns.
	turns, err := ParseSessionTurns(session.SessionLog)
	if err != nil {
		w.logger.Error("failed to parse session", "session_id", session.SessionID, "error", err)
		return
	}

	if len(turns) == 0 {
		return
	}

	// Only process new turns (with overlap for context).
	fromTurn := session.LastTurn - DefaultOverlapTurns
	if fromTurn < 0 {
		fromTurn = 0
	}
	if fromTurn >= len(turns) {
		return
	}
	turnsToProcess := turns[fromTurn:]

	// Chunk.
	chunks := ChunkTurns(turnsToProcess, DefaultTurnsPerChunk, DefaultOverlapTurns)
	if len(chunks) == 0 {
		return
	}

	// Get batch ID.
	batchID, err := w.stateDB.NextBatchID(session.SessionID)
	if err != nil {
		w.logger.Error("failed to get batch ID", "error", err)
		return
	}

	w.logger.Info("extracting observations",
		"session_id", session.SessionID,
		"batch", batchID,
		"chunks", len(chunks),
		"turns", fmt.Sprintf("%d-%d", fromTurn, len(turns)),
	)

	totalObs := 0

	for _, chunk := range chunks {
		// Adjust turn indices to absolute positions.
		absStart := fromTurn + chunk.TurnStart
		absEnd := fromTurn + chunk.TurnEnd

		// Save chunk to state DB.
		chunkID, err := w.stateDB.SaveChunk(
			session.SessionID, batchID,
			absStart, absEnd,
			len(chunk.Text), chunk.Text,
		)
		if err != nil {
			w.logger.Error("failed to save chunk", "error", err)
			continue
		}

		// Extract observations via ollama.
		observations, err := ExtractObservations(ctx, client, chunk)
		if err != nil {
			w.logger.Warn("extraction failed for chunk",
				"chunk", chunk.Index,
				"error", err,
			)
			continue
		}

		// Save observations to state DB.
		for _, obs := range observations {
			_, err := w.stateDB.SaveObservation(
				session.SessionID, chunkID,
				obs.Type, obs.Title, obs.Content,
				obs.Scope, obs.Project, obs.TopicKey,
			)
			if err != nil {
				w.logger.Error("failed to save observation", "title", obs.Title, "error", err)
				continue
			}
			totalObs++
		}

		w.logger.Debug("chunk processed",
			"chunk", chunk.Index,
			"observations", len(observations),
		)
	}

	// Save pending observations to engram.
	saved := 0
	pending, err := w.stateDB.GetPendingObservations(session.SessionID)
	if err != nil {
		w.logger.Error("failed to get pending observations", "error", err)
	} else {
		for _, p := range pending {
			obs := Observation{
				Type:     p.Type,
				Title:    p.Title,
				Content:  p.Content,
				Scope:    p.Scope,
				Project:  p.Project,
				TopicKey: p.TopicKey,
			}
			if err := SaveToEngram(ctx, w.engramAPI, session.SessionID, obs); err != nil {
				w.logger.Warn("failed to save to engram", "title", p.Title, "error", err)
				continue
			}
			if err := w.stateDB.MarkObservationSaved(p.ID); err != nil {
				w.logger.Warn("failed to mark observation saved", "id", p.ID, "error", err)
			}
			saved++
		}
	}

	// Update last extracted turn.
	if err := w.stateDB.UpdateLastTurn(session.SessionID, totalTurns); err != nil {
		w.logger.Error("failed to update last_turn", "error", err)
	}

	w.logger.Info("extraction complete",
		"session_id", session.SessionID,
		"batch", batchID,
		"extracted", totalObs,
		"saved", saved,
	)

	w.stateDB.Log("info", "watcher", session.SessionID,
		fmt.Sprintf("batch %d: %d observations extracted, %d saved to engram", batchID, totalObs, saved))
}

// findSessionLog locates the session JSONL file.
func findSessionLog(sessionID string) string {
	home, _ := os.UserHomeDir()
	projectsDir := filepath.Join(home, ".claude", "projects")

	// Walk projects dir to find the session file.
	var found string
	filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), sessionID+".jsonl") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// countUserTurns counts user messages in a session log (fast grep equivalent).
func countUserTurns(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("reading session log: %w", err)
	}
	return strings.Count(string(data), `"type":"user"`), nil
}
