package ticket

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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

// StateConfig describes a single workflow state.
type StateConfig struct {
	// Label is the human-readable name shown in board column headers etc.
	Label string `json:"label"`
	// Open indicates whether tickets in this state are considered "active".
	// Used by `docket list` with no --state flag and the default list view.
	Open bool `json:"open"`
	// Column is the zero-based position of this state on the kanban board.
	Column int `json:"column"`
	// Next is the list of states this state can transition to.
	Next []string `json:"next"`
}

type Config struct {
	Counter         int                      `json:"counter"`
	Backend         string                   `json:"backend"`
	States          map[string]StateConfig   `json:"states"`
	Labels          []string                 `json:"labels"`
	CommitSessions  bool                     `json:"commit_sessions"`
	DefaultState    string                   `json:"default_state"`
	DefaultPriority int                      `json:"default_priority"`
	HandoffSections []string                 `json:"handoff_sections"`
	Backends        map[string]BackendConfig `json:"backends,omitempty"`
	Semantic        SemanticConfig           `json:"semantic,omitempty"`
}

// defaultStates is the canonical workflow shipped with docket.
var defaultStates = map[string]StateConfig{
	"backlog":     {Label: "Backlog", Open: true, Column: 0, Next: []string{"todo", "archived"}},
	"todo":        {Label: "To Do", Open: true, Column: 1, Next: []string{"in-progress", "backlog", "archived"}},
	"in-progress": {Label: "In Progress", Open: true, Column: 2, Next: []string{"in-review", "todo", "archived"}},
	"in-review":   {Label: "In Review", Open: true, Column: 3, Next: []string{"done", "in-progress", "archived"}},
	"done":        {Label: "Done", Open: false, Column: 4, Next: []string{"archived", "in-progress"}},
	"archived":    {Label: "Archived", Open: false, Column: 5, Next: []string{"backlog"}},
}

var defaultHandoffSections = []string{
	"Current state",
	"Decisions made",
	"Files touched",
	"Remaining work",
	"AC status",
}

func DefaultConfig() *Config {
	states := make(map[string]StateConfig, len(defaultStates))
	for k, v := range defaultStates {
		states[k] = v
	}
	return &Config{
		Counter:         0,
		Backend:         "local",
		States:          states,
		Labels:          []string{"bug", "feature", "refactor", "chore", "llm-only", "human-only"},
		CommitSessions:  false,
		DefaultState:    "backlog",
		DefaultPriority: 10,
		HandoffSections: append([]string(nil), defaultHandoffSections...),
		Backends:        map[string]BackendConfig{},
		Semantic:        defaultSemanticConfig(),
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

// StateNames returns state keys sorted by their Column value.
func (c *Config) StateNames() []string {
	ordered := c.ColumnOrder()
	names := make([]string, len(ordered))
	for i, sc := range ordered {
		// find key for this StateConfig
		for k, v := range c.States {
			if v.Column == sc.Column && v.Label == sc.Label {
				names[i] = k
				break
			}
		}
	}
	return names
}

// OpenStates returns state keys where Open == true, sorted by Column.
func (c *Config) OpenStates() []string {
	var open []string
	for k, v := range c.States {
		if v.Open {
			open = append(open, k)
		}
	}
	sort.Slice(open, func(i, j int) bool {
		return c.States[open[i]].Column < c.States[open[j]].Column
	})
	return open
}

// IsValidState reports whether s is a configured state name.
func (c *Config) IsValidState(s string) bool {
	_, ok := c.States[s]
	return ok
}

// ValidTransitions returns the list of states that from can transition to.
func (c *Config) ValidTransitions(from string) []string {
	sc, ok := c.States[from]
	if !ok {
		return nil
	}
	return sc.Next
}

// ColumnOrder returns all StateConfigs sorted by their Column value.
func (c *Config) ColumnOrder() []StateConfig {
	configs := make([]StateConfig, 0, len(c.States))
	for _, sc := range c.States {
		configs = append(configs, sc)
	}
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Column < configs[j].Column
	})
	return configs
}

func ConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".docket", "config.json")
}

// rawConfigForLoad is used only during loading to detect the states field format.
type rawConfigForLoad struct {
	Counter         int                      `json:"counter"`
	Backend         string                   `json:"backend"`
	States          json.RawMessage          `json:"states"`
	Labels          []string                 `json:"labels"`
	CommitSessions  bool                     `json:"commit_sessions"`
	DefaultState    string                   `json:"default_state"`
	DefaultPriority int                      `json:"default_priority"`
	HandoffSections []string                 `json:"handoff_sections"`
	Backends        map[string]BackendConfig `json:"backends,omitempty"`
	Semantic        SemanticConfig           `json:"semantic,omitempty"`
}

func LoadConfig(repoRoot string) (*Config, error) {
	data, err := os.ReadFile(ConfigPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("docket not initialized in %s — run `docket init`", repoRoot)
		}
		return nil, err
	}

	var raw rawConfigForLoad
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("corrupt config.json: %w", err)
	}

	cfg := &Config{
		Counter:         raw.Counter,
		Backend:         raw.Backend,
		Labels:          raw.Labels,
		CommitSessions:  raw.CommitSessions,
		DefaultState:    raw.DefaultState,
		DefaultPriority: raw.DefaultPriority,
		HandoffSections: raw.HandoffSections,
		Backends:        raw.Backends,
		Semantic:        raw.Semantic,
	}

	migrated, err := parseStates(raw.States)
	if err != nil {
		return nil, fmt.Errorf("corrupt config.json states: %w", err)
	}
	cfg.States = migrated

	cfg.applyDefaults()
	if err := cfg.applyEnvOverrides(); err != nil {
		return nil, err
	}

	// Persist migration if states were in the old array format.
	if needsMigration(raw.States) {
		_ = SaveConfig(repoRoot, cfg) // best-effort; ignore write errors
	}

	return cfg, nil
}

// parseStates handles both the legacy []string format and the new map format.
func parseStates(raw json.RawMessage) (map[string]StateConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 {
		return nil, nil
	}

	if trimmed[0] == '[' {
		// Legacy format: array of state names.
		var names []string
		if err := json.Unmarshal(raw, &names); err != nil {
			return nil, err
		}
		return migrateStateNames(names), nil
	}

	// New format: map[string]StateConfig.
	var states map[string]StateConfig
	if err := json.Unmarshal(raw, &states); err != nil {
		return nil, err
	}
	return states, nil
}

// needsMigration reports whether the raw states JSON is in the old array format.
func needsMigration(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) > 0 && trimmed[0] == '['
}

// migrateStateNames converts a legacy []string state list to the new map format.
// Known default states get their canonical config; unknown states get sensible defaults.
func migrateStateNames(names []string) map[string]StateConfig {
	result := make(map[string]StateConfig, len(names))
	for i, name := range names {
		if sc, ok := defaultStates[name]; ok {
			result[name] = sc
		} else {
			result[name] = StateConfig{
				Label:  cases.Title(language.English).String(strings.ReplaceAll(name, "-", " ")),
				Open:   true,
				Column: i,
				Next:   []string{},
			}
		}
	}
	return result
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
		c.States = def.States
	}
	if len(c.Labels) == 0 {
		c.Labels = append([]string(nil), def.Labels...)
	}
	if c.DefaultState == "" {
		c.DefaultState = def.DefaultState
	}
	if c.DefaultPriority == 0 {
		c.DefaultPriority = def.DefaultPriority
	}
	if len(c.HandoffSections) == 0 {
		c.HandoffSections = append([]string(nil), def.HandoffSections...)
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
