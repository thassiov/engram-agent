// Package config provides configuration loading and management for engram-agent.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the agent configuration.
type Config struct {
	MachineID   string      `json:"machine_id"`    // Unique machine identifier.
	Scope       string      `json:"scope"`         // "personal" or "work".
	EngramDB    string      `json:"engram_db"`     // Path to engram's SQLite database.
	EngramAPI   string      `json:"engram_api"`    // Engram HTTP API base URL.
	ListenAddr  string      `json:"listen_addr"`   // HTTP listen address for hook notifications.
	OllamaURL   string      `json:"ollama_url"`    // Ollama API URL for observation extraction.
	OllamaModel string      `json:"ollama_model"`  // Ollama model name for extraction.
	PullFilter  interface{} `json:"pull_filter"`   // "all" or {"types": ["preference", "config"]}.
	Postgres    *PGConfig   `json:"postgres"`      // PostgreSQL connection for sync. Omit to disable sync.
}

// PGConfig holds PostgreSQL connection settings.
type PGConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	SSLMode  string `json:"sslmode"`
}

// DSN builds a PostgreSQL connection string from the config.
func (p *PGConfig) DSN() string {
	port := p.Port
	if port == 0 {
		port = 5432
	}
	sslmode := p.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s tcp_user_timeout=30000",
		p.Host, port, p.Database, p.User, p.Password, sslmode,
	)
}

// SyncEnabled returns true if PostgreSQL sync is configured.
func (c *Config) SyncEnabled() bool {
	return c.Postgres != nil && c.Postgres.Host != "" && c.Postgres.Password != ""
}

// PullFilterTypes returns the list of observation types to filter on pull,
// or nil if all types should be pulled.
func (c *Config) PullFilterTypes() []string {
	switch v := c.PullFilter.(type) {
	case string:
		if v == "all" {
			return nil
		}
	case map[string]interface{}:
		return extractTypeStrings(v)
	}
	return nil
}

// PullsAll returns true if this machine pulls all mutations (no type filter).
func (c *Config) PullsAll() bool {
	return c.PullFilterTypes() == nil
}

// Load reads the configuration from the given path.
func Load(path string) (*Config, error) {
	path = ExpandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if cfg.MachineID == "" {
		return nil, fmt.Errorf("machine_id is required in %s", path)
	}
	if cfg.Scope == "" {
		cfg.Scope = "personal"
	}
	if cfg.EngramDB == "" {
		cfg.EngramDB = "~/.engram/engram.db"
	}
	if cfg.EngramAPI == "" {
		cfg.EngramAPI = "http://127.0.0.1:7437"
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:7438"
	}
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = "http://127.0.0.1:11434"
	}
	if cfg.OllamaModel == "" {
		cfg.OllamaModel = "phi4-mini"
	}
	if cfg.PullFilter == nil {
		cfg.PullFilter = "all"
	}

	cfg.EngramDB = ExpandHome(cfg.EngramDB)

	return &cfg, nil
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "engram", "agent.json")
}

// ExpandHome replaces a leading ~/ with the user's home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// extractTypeStrings extracts string values from a {"types": [...]} map.
func extractTypeStrings(m map[string]interface{}) []string {
	arr, ok := m["types"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, t := range arr {
		if s, ok := t.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
