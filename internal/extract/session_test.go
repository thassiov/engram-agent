package extract

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSessionFile(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	defer f.Close() //nolint:errcheck
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatalf("writing temp file: %v", err)
		}
	}
	return path
}

func TestParseSessionTurns_BasicFlow(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","name":"Read","id":"x"}]}}`,
		`{"type":"user","message":{"role":"user","content":"what file?"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Glob","id":"y"}]}}`,
	}
	path := writeSessionFile(t, lines)

	turns, err := ParseSessionTurns(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}

	if turns[0].UserText != "hello" {
		t.Errorf("expected turns[0].UserText='hello', got %q", turns[0].UserText)
	}
	if len(turns[0].Tools) != 1 || turns[0].Tools[0] != "Read" {
		t.Errorf("expected turns[0].Tools=['Read'], got %v", turns[0].Tools)
	}

	if turns[1].UserText != "what file?" {
		t.Errorf("expected turns[1].UserText='what file?', got %q", turns[1].UserText)
	}
	if len(turns[1].Tools) != 1 || turns[1].Tools[0] != "Glob" {
		t.Errorf("expected turns[1].Tools=['Glob'], got %v", turns[1].Tools)
	}
}

func TestParseSessionTurns_EmptyFile(t *testing.T) {
	path := writeSessionFile(t, nil)

	turns, err := ParseSessionTurns(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turns) != 0 {
		t.Errorf("expected empty turns slice, got %d turns", len(turns))
	}
}

func TestParseSessionTurns_MalformedLines(t *testing.T) {
	lines := []string{
		`not json at all`,
		`{"type":"user","message":{"role":"user","content":"valid message"}}`,
		`{broken`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","id":"z"}]}}`,
	}
	path := writeSessionFile(t, lines)

	turns, err := ParseSessionTurns(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn from valid lines, got %d", len(turns))
	}
	if turns[0].UserText != "valid message" {
		t.Errorf("expected turns[0].UserText='valid message', got %q", turns[0].UserText)
	}
	if len(turns[0].Tools) != 1 || turns[0].Tools[0] != "Edit" {
		t.Errorf("expected turns[0].Tools=['Edit'], got %v", turns[0].Tools)
	}
}

func TestParseSessionTurns_FiltersEmptyText(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"role":"user","content":""}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ignored"}]}}`,
		`{"type":"user","message":{"role":"user","content":"   "}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"also ignored"}]}}`,
		`{"type":"user","message":{"role":"user","content":"real message"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"response"}]}}`,
	}
	path := writeSessionFile(t, lines)

	turns, err := ParseSessionTurns(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn after filtering empty text, got %d", len(turns))
	}
	if turns[0].UserText != "real message" {
		t.Errorf("expected turns[0].UserText='real message', got %q", turns[0].UserText)
	}
}
