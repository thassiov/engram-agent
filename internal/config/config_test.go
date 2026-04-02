package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPGConfig_DSN_Basic(t *testing.T) {
	pg := &PGConfig{
		Host:     "postgres.grid.local",
		Port:     5432,
		Database: "knowledge",
		User:     "engram",
		Password: "secret",
		SSLMode:  "require",
	}
	dsn := pg.DSN()
	if !strings.Contains(dsn, "host=postgres.grid.local") {
		t.Errorf("DSN missing host: %s", dsn)
	}
	if !strings.Contains(dsn, "port=5432") {
		t.Errorf("DSN missing port: %s", dsn)
	}
	if !strings.Contains(dsn, "dbname=knowledge") {
		t.Errorf("DSN missing dbname: %s", dsn)
	}
	if !strings.Contains(dsn, "user=engram") {
		t.Errorf("DSN missing user: %s", dsn)
	}
	if !strings.Contains(dsn, "password=secret") {
		t.Errorf("DSN missing password: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("DSN missing sslmode: %s", dsn)
	}
}

func TestPGConfig_DSN_Defaults(t *testing.T) {
	pg := &PGConfig{
		Host:     "localhost",
		Database: "mydb",
		User:     "user",
		Password: "pass",
		// Port zero, SSLMode empty — should get defaults.
	}
	dsn := pg.DSN()
	if !strings.Contains(dsn, "port=5432") {
		t.Errorf("expected default port 5432, got DSN: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("expected default sslmode=disable, got DSN: %s", dsn)
	}
}

func TestSyncEnabled_True(t *testing.T) {
	cfg := &Config{
		Postgres: &PGConfig{
			Host:     "postgres.grid.local",
			Password: "secret",
		},
	}
	if !cfg.SyncEnabled() {
		t.Error("expected SyncEnabled true")
	}
}

func TestSyncEnabled_NilPostgres(t *testing.T) {
	cfg := &Config{Postgres: nil}
	if cfg.SyncEnabled() {
		t.Error("expected SyncEnabled false when postgres is nil")
	}
}

func TestSyncEnabled_EmptyHost(t *testing.T) {
	cfg := &Config{
		Postgres: &PGConfig{
			Host:     "",
			Password: "secret",
		},
	}
	if cfg.SyncEnabled() {
		t.Error("expected SyncEnabled false when host is empty")
	}
}

func TestSyncEnabled_EmptyPassword(t *testing.T) {
	cfg := &Config{
		Postgres: &PGConfig{
			Host:     "postgres.grid.local",
			Password: "",
		},
	}
	if cfg.SyncEnabled() {
		t.Error("expected SyncEnabled false when password is empty")
	}
}

func TestEmbedEnabled(t *testing.T) {
	cfg := &Config{EmbedURL: "http://embed.local:8080"}
	if !cfg.EmbedEnabled() {
		t.Error("expected EmbedEnabled true")
	}
	cfg.EmbedURL = ""
	if cfg.EmbedEnabled() {
		t.Error("expected EmbedEnabled false when EmbedURL is empty")
	}
}

func TestPullFilterTypes_All(t *testing.T) {
	cfg := &Config{PullFilter: "all"}
	types := cfg.PullFilterTypes()
	if types != nil {
		t.Errorf("expected nil for 'all', got %v", types)
	}
}

func TestPullFilterTypes_Map(t *testing.T) {
	cfg := &Config{
		PullFilter: map[string]interface{}{
			"types": []interface{}{"preference", "config"},
		},
	}
	types := cfg.PullFilterTypes()
	if len(types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(types))
	}
	if types[0] != "preference" || types[1] != "config" {
		t.Errorf("unexpected types: %v", types)
	}
}

func TestPullFilterTypes_Nil(t *testing.T) {
	cfg := &Config{PullFilter: nil}
	types := cfg.PullFilterTypes()
	if types != nil {
		t.Errorf("expected nil for nil PullFilter, got %v", types)
	}
}

func TestPullsAll(t *testing.T) {
	cfg := &Config{PullFilter: "all"}
	if !cfg.PullsAll() {
		t.Error("expected PullsAll true for 'all'")
	}
	cfg.PullFilter = map[string]interface{}{
		"types": []interface{}{"preference"},
	}
	if cfg.PullsAll() {
		t.Error("expected PullsAll false when type filter is set")
	}
}

func TestExpandHome_ReplacesTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := ExpandHome("~/foo/bar")
	expected := filepath.Join(home, "foo/bar")
	if result != expected {
		t.Errorf("ExpandHome: want %q, got %q", expected, result)
	}
}

func TestExpandHome_AbsolutePath(t *testing.T) {
	abs := "/absolute/path/to/file"
	result := ExpandHome(abs)
	if result != abs {
		t.Errorf("ExpandHome should not modify absolute paths: got %q", result)
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	cfg := map[string]interface{}{
		"machine_id": "testmachine",
		"scope":      "personal",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tmpFile := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.MachineID != "testmachine" {
		t.Errorf("machine_id: want testmachine, got %s", loaded.MachineID)
	}
	// Verify defaults are applied.
	if loaded.EngramAPI != "http://127.0.0.1:7437" {
		t.Errorf("expected default EngramAPI, got %s", loaded.EngramAPI)
	}
	if loaded.OllamaModel != "phi4-mini" {
		t.Errorf("expected default OllamaModel, got %s", loaded.OllamaModel)
	}
	if loaded.EmbedDims != 768 {
		t.Errorf("expected default EmbedDims 768, got %d", loaded.EmbedDims)
	}
	if loaded.DedupThreshold != 0.85 {
		t.Errorf("expected default DedupThreshold 0.85, got %f", loaded.DedupThreshold)
	}
	if loaded.PullFilter != "all" {
		t.Errorf("expected default PullFilter 'all', got %v", loaded.PullFilter)
	}
}

func TestLoad_MissingMachineID(t *testing.T) {
	cfg := map[string]interface{}{
		"scope": "personal",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tmpFile := filepath.Join(t.TempDir(), "agent.json")
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = Load(tmpFile)
	if err == nil {
		t.Error("expected error for missing machine_id, got nil")
	}
	if !strings.Contains(err.Error(), "machine_id") {
		t.Errorf("expected error to mention machine_id, got: %v", err)
	}
}
