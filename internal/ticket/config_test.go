package ticket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigSemanticDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Semantic.Provider != "uv" {
		t.Fatalf("provider = %q", cfg.Semantic.Provider)
	}
	if cfg.Semantic.Model == "" {
		t.Fatal("expected default semantic model")
	}
	if !strings.Contains(cfg.Semantic.HFHome, filepath.Join(".cache", "docket", "hf")) {
		t.Fatalf("hf home = %q", cfg.Semantic.HFHome)
	}
	if cfg.Semantic.LexicalWeight != 0.35 || cfg.Semantic.VectorWeight != 0.65 {
		t.Fatalf("unexpected weights: %+v", cfg.Semantic)
	}
	if cfg.Semantic.TitleWeight != 3.0 || cfg.Semantic.DescriptionWeight != 1.5 || cfg.Semantic.ACWeight != 2.0 || cfg.Semantic.HandoffWeight != 1.25 {
		t.Fatalf("unexpected field weights: %+v", cfg.Semantic)
	}
}

func TestDefaultConfigStateDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultState != "backlog" {
		t.Fatalf("default_state = %q, want backlog", cfg.DefaultState)
	}
	if cfg.DefaultPriority != 10 {
		t.Fatalf("default_priority = %d, want 10", cfg.DefaultPriority)
	}
	if len(cfg.HandoffSections) == 0 {
		t.Fatal("expected handoff_sections to be populated")
	}
	if len(cfg.States) != 6 {
		t.Fatalf("expected 6 default states, got %d", len(cfg.States))
	}

	backlog, ok := cfg.States["backlog"]
	if !ok {
		t.Fatal("missing backlog state")
	}
	if !backlog.Open {
		t.Error("backlog should be open")
	}
	if backlog.Column != 0 {
		t.Errorf("backlog column = %d, want 0", backlog.Column)
	}

	done, ok := cfg.States["done"]
	if !ok {
		t.Fatal("missing done state")
	}
	if done.Open {
		t.Error("done should not be open")
	}
}

// TestLoadConfigMigratesOldArrayFormat verifies that a config with the legacy
// []string states format is auto-migrated to the new map format on load.
func TestLoadConfigMigratesOldArrayFormat(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":1,"backend":"local","states":["backlog","todo","in-progress","in-review","done","archived"],"labels":["bug"],"commit_sessions":false}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// States should be migrated to map format.
	if len(cfg.States) != 6 {
		t.Fatalf("expected 6 states after migration, got %d", len(cfg.States))
	}
	if _, ok := cfg.States["backlog"]; !ok {
		t.Error("backlog state missing after migration")
	}
	if cfg.States["backlog"].Open != true {
		t.Error("backlog should be open after migration")
	}

	// Verify the on-disk config was migrated (saved in new format).
	data, err := os.ReadFile(ConfigPath(tmpDir))
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	var onDisk map[string]json.RawMessage
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("parse migrated config: %v", err)
	}
	statesRaw := strings.TrimSpace(string(onDisk["states"]))
	if len(statesRaw) == 0 || statesRaw[0] != '{' {
		t.Errorf("migrated config states should be object, got: %s", statesRaw)
	}
}

// TestLoadConfigNewMapFormat verifies that a config already in the new map format
// loads without errors.
func TestLoadConfigNewMapFormat(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := SaveConfig(tmpDir, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(cfg.States) != 6 {
		t.Fatalf("expected 6 states, got %d", len(cfg.States))
	}
}

// TestLoadConfigMigratesUnknownState verifies that unknown state names in the
// old array format get sensible StateConfig defaults.
func TestLoadConfigMigratesUnknownState(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":0,"backend":"local","states":["backlog","stale","done"],"labels":[]}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	stale, ok := cfg.States["stale"]
	if !ok {
		t.Fatal("stale state missing")
	}
	if stale.Label == "" {
		t.Error("stale state should have a label")
	}
}

// TestConfigHelpers validates StateNames, OpenStates, IsValidState, ValidTransitions, ColumnOrder.
func TestConfigHelpers(t *testing.T) {
	cfg := DefaultConfig()

	// IsValidState
	if !cfg.IsValidState("backlog") {
		t.Error("backlog should be valid")
	}
	if cfg.IsValidState("nonexistent") {
		t.Error("nonexistent should not be valid")
	}

	// OpenStates: should include backlog, todo, in-progress, in-review; exclude done, archived
	open := cfg.OpenStates()
	openSet := make(map[string]bool, len(open))
	for _, s := range open {
		openSet[s] = true
	}
	for _, s := range []string{"backlog", "todo", "in-progress", "in-review"} {
		if !openSet[s] {
			t.Errorf("expected %q in open states", s)
		}
	}
	for _, s := range []string{"done", "archived"} {
		if openSet[s] {
			t.Errorf("did not expect %q in open states", s)
		}
	}

	// OpenStates should be sorted by column.
	for i := 1; i < len(open); i++ {
		if cfg.States[open[i]].Column < cfg.States[open[i-1]].Column {
			t.Errorf("OpenStates not sorted by column at index %d", i)
		}
	}

	// ValidTransitions
	next := cfg.ValidTransitions("backlog")
	if len(next) == 0 {
		t.Error("expected transitions from backlog")
	}
	found := false
	for _, s := range next {
		if s == "todo" {
			found = true
		}
	}
	if !found {
		t.Error("expected backlog -> todo transition")
	}
	if cfg.ValidTransitions("nonexistent") != nil {
		t.Error("expected nil for unknown state")
	}

	// ColumnOrder: should return all states sorted by Column.
	cols := cfg.ColumnOrder()
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(cols))
	}
	for i := 1; i < len(cols); i++ {
		if cols[i].Column < cols[i-1].Column {
			t.Errorf("ColumnOrder not sorted at index %d", i)
		}
	}

	// StateNames: should return keys sorted by Column.
	names := cfg.StateNames()
	if len(names) != 6 {
		t.Fatalf("expected 6 state names, got %d", len(names))
	}
	if names[0] != "backlog" {
		t.Errorf("first state should be backlog (column 0), got %q", names[0])
	}
}

// TestDefaultStateAndPriorityApplyDefaults verifies missing fields get defaults.
func TestDefaultStateAndPriorityApplyDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	// Config without default_state or default_priority.
	raw := `{"counter":0,"backend":"local","states":{"backlog":{"label":"Backlog","open":true,"column":0,"next":["todo"]}},"labels":[]}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DefaultState != "backlog" {
		t.Errorf("default_state = %q, want backlog", cfg.DefaultState)
	}
	if cfg.DefaultPriority != 10 {
		t.Errorf("default_priority = %d, want 10", cfg.DefaultPriority)
	}
	if len(cfg.HandoffSections) == 0 {
		t.Error("handoff_sections should be defaulted")
	}
}

func TestLoadConfigAppliesSemanticDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	raw := `{"counter":1,"backend":"local","states":["backlog"],"labels":["bug"],"commit_sessions":false}`
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ConfigPath(tmpDir), []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Semantic.Provider != "uv" {
		t.Fatalf("expected default provider, got %q", cfg.Semantic.Provider)
	}
	if cfg.Semantic.Model == "" || cfg.Semantic.HFHome == "" || cfg.Semantic.UVCacheDir == "" {
		t.Fatalf("expected semantic defaults, got %+v", cfg.Semantic)
	}
}

func TestLoadConfigSemanticEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Semantic.Provider = "config-provider"
	cfg.Semantic.Model = "config-model"
	cfg.Semantic.HFHome = "/config/hf"
	cfg.Semantic.LexicalWeight = 0.2
	cfg.Semantic.VectorWeight = 0.8
	cfg.Semantic.TitleWeight = 4
	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	t.Setenv("DOCKET_SEMANTIC_ENABLED", "true")
	t.Setenv("DOCKET_SEMANTIC_PROVIDER", "uv")
	t.Setenv("DOCKET_SEMANTIC_MODEL", "env-model")
	t.Setenv("DOCKET_SEMANTIC_HF_HOME", "/env/hf")
	t.Setenv("DOCKET_SEMANTIC_SENTENCE_TRANSFORMERS_HOME", "/env/sbert")
	t.Setenv("DOCKET_SEMANTIC_UV_CACHE_DIR", "/env/uv")
	t.Setenv("DOCKET_SEMANTIC_LEXICAL_WEIGHT", "0.4")
	t.Setenv("DOCKET_SEMANTIC_VECTOR_WEIGHT", "0.6")
	t.Setenv("DOCKET_SEMANTIC_TITLE_WEIGHT", "5")
	t.Setenv("DOCKET_SEMANTIC_DESCRIPTION_WEIGHT", "1.2")
	t.Setenv("DOCKET_SEMANTIC_AC_WEIGHT", "2.4")
	t.Setenv("DOCKET_SEMANTIC_HANDOFF_WEIGHT", "0.9")

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !loaded.Semantic.Enabled {
		t.Fatal("expected enabled override")
	}
	if loaded.Semantic.Provider != "uv" || loaded.Semantic.Model != "env-model" {
		t.Fatalf("unexpected provider/model: %+v", loaded.Semantic)
	}
	if loaded.Semantic.HFHome != "/env/hf" || loaded.Semantic.SentenceTransformersHome != "/env/sbert" || loaded.Semantic.UVCacheDir != "/env/uv" {
		t.Fatalf("unexpected cache paths: %+v", loaded.Semantic)
	}
	if loaded.Semantic.LexicalWeight != 0.4 || loaded.Semantic.VectorWeight != 0.6 {
		t.Fatalf("unexpected weights: %+v", loaded.Semantic)
	}
	if loaded.Semantic.TitleWeight != 5 || loaded.Semantic.DescriptionWeight != 1.2 || loaded.Semantic.ACWeight != 2.4 || loaded.Semantic.HandoffWeight != 0.9 {
		t.Fatalf("unexpected field weights: %+v", loaded.Semantic)
	}
}

func TestLoadConfigSemanticInvalidEnv(t *testing.T) {
	tmpDir := t.TempDir()
	if err := SaveConfig(tmpDir, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	t.Setenv("DOCKET_SEMANTIC_VECTOR_WEIGHT", "bad")

	_, err := LoadConfig(tmpDir)
	if err == nil || !strings.Contains(err.Error(), "DOCKET_SEMANTIC_VECTOR_WEIGHT") {
		t.Fatalf("expected env parse error, got %v", err)
	}
}
