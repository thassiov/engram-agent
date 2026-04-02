package extract

import (
	"strings"
	"testing"
)

func TestChunkTurns_Empty(t *testing.T) {
	result := ChunkTurns(nil, DefaultTurnsPerChunk, DefaultOverlapTurns)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestChunkTurns_FewerThanChunkSize(t *testing.T) {
	turns := []Turn{
		{UserText: "hello", Tools: nil},
		{UserText: "world", Tools: nil},
	}
	chunks := ChunkTurns(turns, DefaultTurnsPerChunk, DefaultOverlapTurns)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].TurnStart != 0 || chunks[0].TurnEnd != 2 {
		t.Errorf("unexpected chunk bounds: start=%d end=%d", chunks[0].TurnStart, chunks[0].TurnEnd)
	}
}

func TestChunkTurns_MultipleChunks(t *testing.T) {
	// 12 turns with chunk size 5 and overlap 2 should produce multiple chunks.
	turns := make([]Turn, 12)
	for i := range turns {
		turns[i] = Turn{UserText: "turn"}
	}
	chunks := ChunkTurns(turns, 5, 2)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	// All turns should be covered.
	for i, c := range chunks {
		if c.TurnStart < 0 || c.TurnEnd > len(turns) {
			t.Errorf("chunk %d out of bounds: start=%d end=%d", i, c.TurnStart, c.TurnEnd)
		}
	}
}

func TestChunkTurns_OverlapCorrectness(t *testing.T) {
	// With 10 turns, chunk size 5, overlap 2:
	// chunk 0: [0,5), chunk 1 starts at 5-2=3
	turns := make([]Turn, 10)
	for i := range turns {
		turns[i] = Turn{UserText: "turn"}
	}
	chunks := ChunkTurns(turns, 5, 2)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	// Second chunk should start at first chunk end minus overlap.
	expectedStart := chunks[0].TurnEnd - 2
	if chunks[1].TurnStart != expectedStart {
		t.Errorf("second chunk start: want %d, got %d", expectedStart, chunks[1].TurnStart)
	}
}

func TestFormatChunk_Basic(t *testing.T) {
	turns := []Turn{
		{UserText: "hello world", Tools: []string{"Read", "Edit"}},
	}
	text := formatChunk(turns)
	if !strings.Contains(text, "USER: hello world") {
		t.Errorf("expected USER prefix, got: %s", text)
	}
	if !strings.Contains(text, "Tools used: Read, Edit") {
		t.Errorf("expected tools line, got: %s", text)
	}
}

func TestFormatChunk_NoTools(t *testing.T) {
	turns := []Turn{
		{UserText: "just a message", Tools: nil},
	}
	text := formatChunk(turns)
	if !strings.Contains(text, "USER: just a message") {
		t.Errorf("expected user text, got: %s", text)
	}
	if strings.Contains(text, "Tools used") {
		t.Errorf("expected no tools line when no tools, got: %s", text)
	}
}

func TestFormatChunk_MultipleTurns(t *testing.T) {
	turns := []Turn{
		{UserText: "first", Tools: nil},
		{UserText: "second", Tools: nil},
	}
	text := formatChunk(turns)
	if !strings.Contains(text, "USER: first") || !strings.Contains(text, "USER: second") {
		t.Errorf("expected both turns in output, got: %s", text)
	}
	// Should be separated by double newline.
	if !strings.Contains(text, "\n\n") {
		t.Errorf("expected double newline separator between turns, got: %s", text)
	}
}
