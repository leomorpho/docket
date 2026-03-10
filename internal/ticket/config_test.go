package ticket

import (
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
