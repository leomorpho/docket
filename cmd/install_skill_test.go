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

	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install", "--skill"})

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
		"ticket to 'in-review'",
		"merge them back to the main branch",
		"prune the linked worktree",
		"human reviewer advances the ticket to 'done'",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(string(content), s) {
			t.Errorf("SKILL.md missing expected string: %s", s)
		}
	}
}

func TestInstallCursorRulesIncludesReviewMergeWorkflow(t *testing.T) {
	tmpHome := t.TempDir()
	tmpRepo := t.TempDir()
	originalHome := os.Getenv("HOME")
	originalRepo := repo
	os.Setenv("HOME", tmpHome)
	repo = tmpRepo
	defer os.Setenv("HOME", originalHome)
	defer func() { repo = originalRepo }()

	installSkill = false
	installCursor = false
	installVSCode = false

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install", "--cursor"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install --cursor failed: %v", err)
	}

	rulesPath := filepath.Join(tmpRepo, ".cursorrules")
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("failed to read .cursorrules: %v", err)
	}

	expectedStrings := []string{
		"ticket to 'in-review'",
		"merge them back to the main branch",
		"prune the linked worktree",
		"human reviewer advances the ticket to 'done'",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(strings.ToLower(string(content)), strings.ToLower(s)) {
			t.Errorf(".cursorrules missing expected string: %s", s)
		}
	}
}
