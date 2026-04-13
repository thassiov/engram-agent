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

// SaveResult holds the response from the engram API after saving an observation.
type SaveResult struct {
	ID int64 `json:"id"`
}

// SaveToEngram posts an observation to the engram HTTP API and returns the created observation ID.
func SaveToEngram(ctx context.Context, baseURL, sessionID string, obs Observation) (*SaveResult, error) {
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
		return nil, fmt.Errorf("marshaling observation: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/observations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending observation: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusConflict {
		var result SaveResult
		if err := json.Unmarshal(respBody, &result); err != nil {
			// API saved but didn't return ID — not fatal.
			return &SaveResult{}, nil
		}
		return &result, nil
	}
	return nil, fmt.Errorf("unexpected status %d from engram API", resp.StatusCode)
}
