package ticket

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type BackendConfig map[string]interface{}

type Config struct {
	Counter        int                      `json:"counter"`
	Backend        string                   `json:"backend"`
	States         []string                 `json:"states"`
	Labels         []string                 `json:"labels"`
	CommitSessions bool                     `json:"commit_sessions"`
	Backends       map[string]BackendConfig `json:"backends,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		Counter: 0,
		Backend: "local",
		States:  []string{"backlog", "todo", "in-progress", "in-review", "done", "archived"},
		Labels:  []string{"bug", "feature", "refactor", "chore", "llm-only", "human-only"},
		CommitSessions: false,
		Backends:       map[string]BackendConfig{},
	}
}

func ConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "config.json")
}

func LoadConfig(repoRoot string) (*Config, error) {
	data, err := os.ReadFile(ConfigPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("docket not initialized in %s — run `docket init`", repoRoot)
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("corrupt config.json: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(repoRoot string, cfg *Config) error {
	dir := filepath.Dir(ConfigPath(repoRoot))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(repoRoot), append(data, '\n'), 0644)
}
