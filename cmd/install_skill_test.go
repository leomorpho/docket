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
	tmpRepo := t.TempDir()
	originalHome := os.Getenv("HOME")
	originalCodexHome := os.Getenv("CODEX_HOME")
	originalRepo := repo
	os.Setenv("HOME", tmpHome)
	os.Setenv("CODEX_HOME", filepath.Join(tmpHome, ".codex-home"))
	repo = tmpRepo
	t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "docket-home"))
	defer os.Setenv("HOME", originalHome)
	defer os.Setenv("CODEX_HOME", originalCodexHome)
	defer func() { repo = originalRepo }()

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

	for _, p := range []string{
		filepath.Join(tmpRepo, "AGENTS.md"),
		filepath.Join(tmpRepo, "CLAUDE.md"),
		filepath.Join(tmpRepo, "GEMINI.md"),
		filepath.Join(tmpRepo, "doombox.json"),
		filepath.Join(tmpRepo, ".cursor", "mcp.json"),
		filepath.Join(tmpHome, ".gemini", "skills", "docket", "SKILL.md"),
		filepath.Join(tmpHome, ".gemini", "settings.json"),
		filepath.Join(tmpHome, ".codex-home", "skills", "docket", "SKILL.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected install --skill artifact %s: %v", p, err)
		}
	}

	content, err := os.ReadFile(filepath.Join(tmpHome, ".codex-home", "skills", "docket", "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	expectedStrings := []string{
		"Docket Skill Pack",
		"docket.skill.metadata.checksum",
		"docket.skill.names",
		"ticket-discovery",
		"wrap-up-readiness",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(string(content), s) {
			t.Errorf("SKILL.md missing expected string: %s", s)
		}
	}
}

func TestAutoSkillSyncRepairsMissingOrStaleSkillsOnCommandUsage(t *testing.T) {
	tmpHome := t.TempDir()
	tmpRepo := t.TempDir()
	originalHome := os.Getenv("HOME")
	originalCodexHome := os.Getenv("CODEX_HOME")
	originalRepo := repo
	os.Setenv("HOME", tmpHome)
	os.Setenv("CODEX_HOME", filepath.Join(tmpHome, ".codex-home"))
	t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "docket-home"))
	repo = tmpRepo
	defer os.Setenv("HOME", originalHome)
	defer os.Setenv("CODEX_HOME", originalCodexHome)
	defer func() { repo = originalRepo }()

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md failed: %v", err)
	}

	// First command usage should auto-install skill artifacts.
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{"skill", "list", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("skill list failed: %v\n%s", err, out.String())
	}

	codexSkillPath := filepath.Join(tmpHome, ".codex-home", "skills", "docket", "SKILL.md")
	if _, err := os.Stat(codexSkillPath); err != nil {
		t.Fatalf("expected codex skill after auto-sync: %v", err)
	}

	// Tamper with skill content, then ensure next command rewrites canonical content.
	if err := os.WriteFile(codexSkillPath, []byte("stale-skill-content\n"), 0o644); err != nil {
		t.Fatalf("tamper codex skill failed: %v", err)
	}
	out.Reset()
	rootCmd.SetArgs([]string{"skill", "show", "ticket-discovery"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("skill show failed after tamper: %v\n%s", err, out.String())
	}

	updated, err := os.ReadFile(codexSkillPath)
	if err != nil {
		t.Fatalf("read repaired codex skill failed: %v", err)
	}
	if !strings.Contains(string(updated), "docket.skill.metadata.checksum") {
		t.Fatalf("expected repaired codex skill with canonical metadata marker, got:\n%s", string(updated))
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
		"configured active work state",
		"merge them back to the main branch",
		"prune the linked worktree",
		"configured validated/completed state",
		"stay on the managed Docket branch/worktree",
	}

	for _, s := range expectedStrings {
		if !strings.Contains(strings.ToLower(string(content)), strings.ToLower(s)) {
			t.Errorf(".cursorrules missing expected string: %s", s)
		}
	}
}
