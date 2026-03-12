package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkill(t *testing.T) {
	tmpHome := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install", "--skill"})
	
	// Reset flags because they are global
	installSkill = false 

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install --skill failed: %v", err)
	}

	skillPath := filepath.Join(tmpHome, ".gemini", "skills", "docket", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("SKILL.md not found at %s: %v", skillPath, err)
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	expectedStrings := []string{
		"# Docket Skill",
		"MCP server",
		"CRITICAL: Do not edit .docket/tickets/*.md directly",
		"TKT-142/143",
		"TKT-146",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(string(content), s) {
			t.Errorf("SKILL.md missing expected string: %s", s)
		}
	}
}
