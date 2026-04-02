package extract

import (
	"testing"
)

func TestParseObservations_ValidJSONL(t *testing.T) {
	input := `{"type":"decision","title":"Chose SQLite","content":"Used SQLite for local store.","scope":"personal","project":"engram","topic_key":"engram/db"}
{"type":"config","title":"Set up TLP config","content":"Created drop-in config file.","scope":"project","project":"portus","topic_key":"portus/tlp"}`

	obs := parseObservations(input)
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(obs))
	}
	if obs[0].Type != "decision" || obs[0].Title != "Chose SQLite" {
		t.Errorf("first observation mismatch: %+v", obs[0])
	}
	if obs[1].Type != "config" || obs[1].Title != "Set up TLP config" {
		t.Errorf("second observation mismatch: %+v", obs[1])
	}
}

func TestParseObservations_MessyOutput(t *testing.T) {
	input := `Here are the observations I extracted:
` + "```json" + `
{"type":"discovery","title":"Found N+1 query","content":"There was an N+1 in UserList.","scope":"project","project":"myapp","topic_key":"myapp/db"}
` + "```" + `
Some trailing text.`

	obs := parseObservations(input)
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation from messy output, got %d", len(obs))
	}
	if obs[0].Type != "discovery" {
		t.Errorf("expected type discovery, got %s", obs[0].Type)
	}
}

func TestParseObservations_SkipsMissingTypeOrTitle(t *testing.T) {
	// Missing type.
	missingType := `{"title":"Something happened","content":"Details.","scope":"project","project":"x","topic_key":"x/y"}`
	// Missing title.
	missingTitle := `{"type":"decision","content":"Details.","scope":"project","project":"x","topic_key":"x/y"}`
	// Valid one.
	valid := `{"type":"pattern","title":"Use cobra","content":"All CLIs use cobra.","scope":"project","project":"grid","topic_key":"grid/cli"}`

	input := missingType + "\n" + missingTitle + "\n" + valid
	obs := parseObservations(input)
	if len(obs) != 1 {
		t.Fatalf("expected only 1 valid observation, got %d", len(obs))
	}
	if obs[0].Type != "pattern" {
		t.Errorf("expected pattern type, got %s", obs[0].Type)
	}
}

func TestParseObservations_DefaultScopeAndProject(t *testing.T) {
	// No scope, no project fields.
	input := `{"type":"bugfix","title":"Fixed nil pointer","content":"Added nil check."}`

	obs := parseObservations(input)
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].Scope != "project" {
		t.Errorf("expected default scope 'project', got %q", obs[0].Scope)
	}
	if obs[0].Project != "general" {
		t.Errorf("expected default project 'general', got %q", obs[0].Project)
	}
}

func TestParseObservations_InvalidInput(t *testing.T) {
	obs := parseObservations("this is not json at all !!!")
	if len(obs) != 0 {
		t.Errorf("expected empty result for invalid input, got %d observations", len(obs))
	}
}

func TestParseObservations_EmptyString(t *testing.T) {
	obs := parseObservations("")
	if len(obs) != 0 {
		t.Errorf("expected empty result for empty string, got %d", len(obs))
	}
}
