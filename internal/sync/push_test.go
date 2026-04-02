package sync

import (
	"testing"

	"github.com/thassiov/engram-agent/internal/config"
)

func TestScopeFromMutation_ExtractsScope(t *testing.T) {
	m := Mutation{
		Payload: `{"scope":"personal","title":"some obs"}`,
	}
	cfg := &config.Config{Scope: "work"}
	result := scopeFromMutation(m, cfg)
	if result != "personal" {
		t.Errorf("expected 'personal' from payload, got %q", result)
	}
}

func TestScopeFromMutation_FallbackOnMissingField(t *testing.T) {
	m := Mutation{
		Payload: `{"title":"no scope here"}`,
	}
	cfg := &config.Config{Scope: "work"}
	result := scopeFromMutation(m, cfg)
	if result != "work" {
		t.Errorf("expected fallback to config scope 'work', got %q", result)
	}
}

func TestScopeFromMutation_FallbackOnInvalidJSON(t *testing.T) {
	m := Mutation{
		Payload: `not valid json`,
	}
	cfg := &config.Config{Scope: "personal"}
	result := scopeFromMutation(m, cfg)
	if result != "personal" {
		t.Errorf("expected fallback to config scope 'personal' on invalid JSON, got %q", result)
	}
}

func TestScopeFromMutation_FallbackOnEmptyScopeField(t *testing.T) {
	m := Mutation{
		Payload: `{"scope":""}`,
	}
	cfg := &config.Config{Scope: "personal"}
	result := scopeFromMutation(m, cfg)
	if result != "personal" {
		t.Errorf("expected fallback to config scope when payload scope is empty, got %q", result)
	}
}
