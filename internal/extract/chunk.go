package extract

import (
	"fmt"
	"strings"
)

const (
	// DefaultTurnsPerChunk is the number of turns per chunk.
	DefaultTurnsPerChunk = 8
	// DefaultOverlapTurns is the number of overlapping turns between chunks.
	DefaultOverlapTurns = 3
)

// Chunk represents a group of turns ready for extraction.
type Chunk struct {
	Index     int
	TurnStart int
	TurnEnd   int
	Text      string
}

// ChunkTurns splits turns into overlapping chunks for extraction.
func ChunkTurns(turns []Turn, turnsPerChunk, overlap int) []Chunk {
	if len(turns) == 0 {
		return nil
	}

	var chunks []Chunk
	start := 0
	index := 0

	for start < len(turns) {
		end := start + turnsPerChunk
		if end > len(turns) {
			end = len(turns)
		}

		text := formatChunk(turns[start:end])
		chunks = append(chunks, Chunk{
			Index:     index,
			TurnStart: start,
			TurnEnd:   end,
			Text:      text,
		})

		if end >= len(turns) {
			break
		}

		nextStart := end - overlap
		if nextStart <= start {
			nextStart = start + 1
		}
		start = nextStart
		index++
	}

	return chunks
}

// formatChunk formats a slice of turns into text suitable for the LLM.
func formatChunk(turns []Turn) string {
	var sb strings.Builder
	for i, t := range turns {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("USER: ")
		sb.WriteString(t.UserText)
		if len(t.Tools) > 0 {
			sb.WriteString(fmt.Sprintf("\n[Tools used: %s]", strings.Join(t.Tools, ", ")))
		}
	}
	return sb.String()
}
