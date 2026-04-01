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
	MachineID     string      `json:"machine_id"`     // Unique machine identifier.
	Scope         string      `json:"scope"`          // "personal" or "work".
	PGCredentials string      `json:"pg_credentials"` // Path to credentials.env.
	EngramDB      string      `json:"engram_db"`      // Path to engram's SQLite database.
	EngramAPI     string      `json:"engram_api"`     // Engram HTTP API base URL.
	ListenAddr    string      `json:"listen_addr"`    // HTTP listen address for hook notifications.
	PullFilter    interface{} `json:"pull_filter"`    // "all" or {"types": ["preference", "config"]}.
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
	if cfg.PGCredentials == "" {
		cfg.PGCredentials = "~/.config/grid/credentials.env"
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:7438"
	}
	if cfg.PullFilter == nil {
		cfg.PullFilter = "all"
	}

	cfg.EngramDB = ExpandHome(cfg.EngramDB)
	cfg.PGCredentials = ExpandHome(cfg.PGCredentials)

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
