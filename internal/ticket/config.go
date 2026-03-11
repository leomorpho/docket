package ticket

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type BackendConfig map[string]interface{}

type SemanticConfig struct {
	Enabled                  bool    `json:"enabled"`
	Provider                 string  `json:"provider"`
	Model                    string  `json:"model"`
	HFHome                   string  `json:"hf_home,omitempty"`
	SentenceTransformersHome string  `json:"sentence_transformers_home,omitempty"`
	UVCacheDir               string  `json:"uv_cache_dir,omitempty"`
	LexicalWeight            float64 `json:"lexical_weight"`
	VectorWeight             float64 `json:"vector_weight"`
	TitleWeight              float64 `json:"title_weight"`
	DescriptionWeight        float64 `json:"description_weight"`
	ACWeight                 float64 `json:"ac_weight"`
	HandoffWeight            float64 `json:"handoff_weight"`
}

type Config struct {
	Counter        int                      `json:"counter"`
	Backend        string                   `json:"backend"`
	States         []string                 `json:"states"`
	Labels         []string                 `json:"labels"`
	CommitSessions bool                     `json:"commit_sessions"`
	Backends       map[string]BackendConfig `json:"backends,omitempty"`
	Semantic       SemanticConfig           `json:"semantic,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		Counter:        0,
		Backend:        "local",
		States:         []string{"backlog", "todo", "in-progress", "in-review", "done", "archived"},
		Labels:         []string{"bug", "feature", "refactor", "chore", "llm-only", "human-only"},
		CommitSessions: false,
		Backends:       map[string]BackendConfig{},
		Semantic:       defaultSemanticConfig(),
	}
}

func defaultSemanticConfig() SemanticConfig {
	cacheRoot := filepath.Join(userHomeDir(), ".cache", "docket")
	return SemanticConfig{
		Enabled:                  false,
		Provider:                 "uv",
		Model:                    "sentence-transformers/all-MiniLM-L6-v2",
		HFHome:                   filepath.Join(cacheRoot, "hf"),
		SentenceTransformersHome: filepath.Join(cacheRoot, "sbert"),
		UVCacheDir:               filepath.Join(cacheRoot, "uv"),
		LexicalWeight:            0.35,
		VectorWeight:             0.65,
		TitleWeight:              3.0,
		DescriptionWeight:        1.5,
		ACWeight:                 2.0,
		HandoffWeight:            1.25,
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
	cfg.applyDefaults()
	if err := cfg.applyEnvOverrides(); err != nil {
		return nil, err
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

func (c *Config) applyDefaults() {
	def := DefaultConfig()
	if c.Backend == "" {
		c.Backend = def.Backend
	}
	if len(c.States) == 0 {
		c.States = append([]string(nil), def.States...)
	}
	if len(c.Labels) == 0 {
		c.Labels = append([]string(nil), def.Labels...)
	}
	if c.Backends == nil {
		c.Backends = map[string]BackendConfig{}
	}
	if c.Semantic.Provider == "" {
		c.Semantic.Provider = def.Semantic.Provider
	}
	if c.Semantic.Model == "" {
		c.Semantic.Model = def.Semantic.Model
	}
	if c.Semantic.HFHome == "" {
		c.Semantic.HFHome = def.Semantic.HFHome
	}
	if c.Semantic.SentenceTransformersHome == "" {
		c.Semantic.SentenceTransformersHome = def.Semantic.SentenceTransformersHome
	}
	if c.Semantic.UVCacheDir == "" {
		c.Semantic.UVCacheDir = def.Semantic.UVCacheDir
	}
	if c.Semantic.LexicalWeight == 0 {
		c.Semantic.LexicalWeight = def.Semantic.LexicalWeight
	}
	if c.Semantic.VectorWeight == 0 {
		c.Semantic.VectorWeight = def.Semantic.VectorWeight
	}
	if c.Semantic.TitleWeight == 0 {
		c.Semantic.TitleWeight = def.Semantic.TitleWeight
	}
	if c.Semantic.DescriptionWeight == 0 {
		c.Semantic.DescriptionWeight = def.Semantic.DescriptionWeight
	}
	if c.Semantic.ACWeight == 0 {
		c.Semantic.ACWeight = def.Semantic.ACWeight
	}
	if c.Semantic.HandoffWeight == 0 {
		c.Semantic.HandoffWeight = def.Semantic.HandoffWeight
	}
}

func (c *Config) applyEnvOverrides() error {
	applyStringEnv("DOCKET_SEMANTIC_PROVIDER", &c.Semantic.Provider)
	applyStringEnv("DOCKET_SEMANTIC_MODEL", &c.Semantic.Model)
	applyStringEnv("DOCKET_SEMANTIC_HF_HOME", &c.Semantic.HFHome)
	applyStringEnv("DOCKET_SEMANTIC_SENTENCE_TRANSFORMERS_HOME", &c.Semantic.SentenceTransformersHome)
	applyStringEnv("DOCKET_SEMANTIC_UV_CACHE_DIR", &c.Semantic.UVCacheDir)

	if err := applyBoolEnv("DOCKET_SEMANTIC_ENABLED", &c.Semantic.Enabled); err != nil {
		return err
	}
	if err := applyFloatEnv("DOCKET_SEMANTIC_LEXICAL_WEIGHT", &c.Semantic.LexicalWeight); err != nil {
		return err
	}
	if err := applyFloatEnv("DOCKET_SEMANTIC_VECTOR_WEIGHT", &c.Semantic.VectorWeight); err != nil {
		return err
	}
	if err := applyFloatEnv("DOCKET_SEMANTIC_TITLE_WEIGHT", &c.Semantic.TitleWeight); err != nil {
		return err
	}
	if err := applyFloatEnv("DOCKET_SEMANTIC_DESCRIPTION_WEIGHT", &c.Semantic.DescriptionWeight); err != nil {
		return err
	}
	if err := applyFloatEnv("DOCKET_SEMANTIC_AC_WEIGHT", &c.Semantic.ACWeight); err != nil {
		return err
	}
	if err := applyFloatEnv("DOCKET_SEMANTIC_HANDOFF_WEIGHT", &c.Semantic.HandoffWeight); err != nil {
		return err
	}
	return nil
}

func applyStringEnv(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok {
		*target = strings.TrimSpace(value)
	}
}

func applyBoolEnv(key string, target *bool) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("%s: parse bool: %w", key, err)
	}
	*target = parsed
	return nil
}

func applyFloatEnv(key string, target *float64) error {
	value, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fmt.Errorf("%s: parse float: %w", key, err)
	}
	*target = parsed
	return nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "~"
	}
	return home
}
