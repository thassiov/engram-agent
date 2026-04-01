// Package extract handles session JSONL parsing, chunking, and observation extraction.
package extract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Turn represents a single user turn with the tools used in the assistant's response.
type Turn struct {
	UserText string
	Tools    []string
}

// sessionMessage represents a single line from the session JSONL.
type sessionMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type messageBody struct {
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
}

// ParseSessionTurns reads a session JSONL file and extracts user turns.
// Each turn is a user message paired with the tool names from the subsequent assistant response.
func ParseSessionTurns(path string) ([]Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening session log: %w", err)
	}
	defer f.Close() //nolint:errcheck

	var turns []Turn
	var currentUser string
	var currentTools []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var msg sessionMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip malformed lines
		}

		switch msg.Type {
		case "user":
			// Save previous turn if exists.
			if currentUser != "" {
				turns = append(turns, Turn{UserText: currentUser, Tools: currentTools})
			}
			currentUser = extractUserText(msg.Message)
			currentTools = nil

		case "assistant":
			tools := extractToolNames(msg.Message)
			currentTools = append(currentTools, tools...)
		}
	}

	// Save last turn.
	if currentUser != "" {
		turns = append(turns, Turn{UserText: currentUser, Tools: currentTools})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning session log: %w", err)
	}

	// Filter out empty turns.
	filtered := turns[:0]
	for _, t := range turns {
		if strings.TrimSpace(t.UserText) != "" {
			filtered = append(filtered, t)
		}
	}

	return filtered, nil
}

// extractUserText gets the text content from a user message.
// Content can be a string or an array of content blocks.
func extractUserText(raw json.RawMessage) string {
	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}

	// Try as string first.
	var s string
	if err := json.Unmarshal(body.Content, &s); err == nil {
		return s
	}

	// Try as array of content blocks.
	var blocks []contentBlock
	if err := json.Unmarshal(body.Content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}

// extractToolNames gets tool_use names from an assistant message.
func extractToolNames(raw json.RawMessage) []string {
	var body messageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil
	}

	var blocks []contentBlock
	if err := json.Unmarshal(body.Content, &blocks); err != nil {
		return nil
	}

	var names []string
	for _, b := range blocks {
		if b.Type == "tool_use" && b.Name != "" {
			names = append(names, b.Name)
		}
	}
	return names
}
