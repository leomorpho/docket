package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Set global repo flag for the command
	repo = tmpDir
	format = "human"

	// 1. First init
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"init"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if !strings.Contains(b.String(), "Initialized docket") {
		t.Errorf("expected 'Initialized docket' in output, got: %s", b.String())
	}

	cfgPath := filepath.Join(tmpDir, ".docket", "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Errorf("config.json not created at %s", cfgPath)
	}

	ticketsDir := filepath.Join(tmpDir, ".docket", "tickets")
	if _, err := os.Stat(ticketsDir); os.IsNotExist(err) {
		t.Errorf("tickets directory not created at %s", ticketsDir)
	}

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	data, _ := os.ReadFile(gitignorePath)
	if !strings.Contains(string(data), ".docket/index.db") {
		t.Errorf(".gitignore does not contain .docket/index.db")
	}

	// 2. Second init (idempotency)
	b.Reset()
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second init failed: %v", err)
	}
	if !strings.Contains(b.String(), "already initialized") {
		t.Errorf("expected 'already initialized' in output, got: %s", b.String())
	}

	// 3. JSON format
	format = "json"
	b.Reset()
	rootCmd.SetArgs([]string{"init"}) // already initialized but let's see JSON
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("JSON init failed: %v", err)
	}
	var res map[string]string
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}
	if res["status"] != "already initialized" {
		t.Errorf("expected status 'already initialized', got: %s", res["status"])
	}
}
