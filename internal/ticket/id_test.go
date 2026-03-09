package ticket

import (
	"testing"
)

func TestFormatID(t *testing.T) {
	tests := []struct {
		seq      int
		expected string
	}{
		{1, "TKT-001"},
		{42, "TKT-042"},
		{123, "TKT-123"},
		{1000, "TKT-1000"},
	}

	for _, tt := range tests {
		if got := FormatID(tt.seq); got != tt.expected {
			t.Errorf("FormatID(%d) = %q, want %q", tt.seq, got, tt.expected)
		}
	}
}

func TestNextID(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. NextID on missing config should fail
	_, _, err := NextID(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}

	// 2. Initialize config
	cfg := DefaultConfig()
	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// 3. First ID
	id1, seq1, err := NextID(tmpDir)
	if err != nil {
		t.Fatalf("NextID failed: %v", err)
	}
	if id1 != "TKT-001" || seq1 != 1 {
		t.Errorf("expected TKT-001/1, got %s/%d", id1, seq1)
	}

	// 4. Second ID
	id2, seq2, err := NextID(tmpDir)
	if err != nil {
		t.Fatalf("NextID failed: %v", err)
	}
	if id2 != "TKT-002" || seq2 != 2 {
		t.Errorf("expected TKT-002/2, got %s/%d", id2, seq2)
	}

	// 5. Verify persistence
	loaded, _ := LoadConfig(tmpDir)
	if loaded.Counter != 2 {
		t.Errorf("expected counter 2 in config, got %d", loaded.Counter)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Counter = 123
	cfg.Backend = "custom"
	cfg.Backends["jira"] = BackendConfig{"url": "https://jira.example.com"}

	if err := SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Counter != cfg.Counter {
		t.Errorf("Counter mismatch: %d != %d", loaded.Counter, cfg.Counter)
	}
	if loaded.Backend != cfg.Backend {
		t.Errorf("Backend mismatch: %s != %s", loaded.Backend, cfg.Backend)
	}
	jiraCfg, ok := loaded.Backends["jira"]
	if !ok || jiraCfg["url"] != "https://jira.example.com" {
		t.Errorf("Backends mismatch or missing: %v", loaded.Backends)
	}
}
