package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCursor(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()
	
	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false
	
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install", "--cursor"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install --cursor failed: %v", err)
	}

	path := filepath.Join(tmpDir, ".cursorrules")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".cursorrules not found: %v", err)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "Docket Rules for Cursor Agents") {
		t.Errorf(".cursorrules missing expected content")
	}

	// Test idempotency/append
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second install --cursor failed: %v", err)
	}
	content2, _ := os.ReadFile(path)
	if string(content) != string(content2) {
		t.Errorf("install --cursor should be idempotent if rules already present")
	}
}

func TestInstallVSCode(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()
	
	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false
	
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install", "--vscode"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install --vscode failed: %v", err)
	}

	path := filepath.Join(tmpDir, ".vscode", "settings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".vscode/settings.json not found: %v", err)
	}

	content, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(content, &m); err != nil {
		t.Fatalf("invalid json in settings.json: %v", err)
	}

	servers := m["mcp.servers"].(map[string]any)
	if servers["docket"] == nil {
		t.Errorf("docket server not found in settings.json")
	}

	// Test merge
	otherSettings := `{"other": "value"}`
	os.WriteFile(path, []byte(otherSettings), 0644)
	
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("merge install --vscode failed: %v", err)
	}
	
	content, _ = os.ReadFile(path)
	json.Unmarshal(content, &m)
	if m["other"] != "value" {
		t.Errorf("existing settings lost during merge")
	}
	if m["mcp.servers"].(map[string]any)["docket"] == nil {
		t.Errorf("docket server missing after merge")
	}
}
