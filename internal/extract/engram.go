package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type engramObservation struct {
	SessionID string `json:"session_id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Scope     string `json:"scope"`
	Project   string `json:"project"`
	TopicKey  string `json:"topic_key"`
	ToolName  string `json:"tool_name"`
}

// SaveToEngram posts an observation to the engram HTTP API.
func SaveToEngram(ctx context.Context, baseURL, sessionID string, obs Observation) error {
	body, err := json.Marshal(engramObservation{
		SessionID: sessionID,
		Type:      obs.Type,
		Title:     obs.Title,
		Content:   obs.Content,
		Scope:     obs.Scope,
		Project:   obs.Project,
		TopicKey:  obs.TopicKey,
		ToolName:  "engram-agent",
	})
	if err != nil {
		return fmt.Errorf("marshaling observation: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/observations", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending observation: %w", err)
	}
	defer resp.Body.Close()               //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body) // drain for connection reuse

	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("unexpected status %d from engram API", resp.StatusCode)
}
