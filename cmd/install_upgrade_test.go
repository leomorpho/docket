package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCreatesManagedArtifactsAndIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpDir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}

	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	hookPath := preCommitHookPath(tmpDir)
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook missing: %v", err)
	}
	if !strings.Contains(string(hookData), "Ticket: TKT-NNN") {
		t.Fatalf("hook content missing ticket warning logic")
	}
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook stat failed: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("hook should be executable, mode=%v", info.Mode())
	}

	manifestData, err := os.ReadFile(installManifestPath(tmpDir))
	if err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(manifestData, &m); err != nil {
		t.Fatalf("manifest invalid json: %v", err)
	}
	if m["docket_version"] == nil {
		t.Fatalf("manifest missing docket_version")
	}

	claudeData, err := os.ReadFile(claudePath(tmpDir))
	if err != nil {
		t.Fatalf("CLAUDE.md missing: %v", err)
	}
	if !strings.Contains(string(claudeData), docketBlockStart) || !strings.Contains(string(claudeData), docketBlockEnd) {
		t.Fatalf("CLAUDE.md missing managed markers")
	}

	before := string(claudeData)
	rootCmd.SetOut(new(bytes.Buffer))
	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false
	rootCmd.SetArgs([]string{"install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	afterData, _ := os.ReadFile(claudePath(tmpDir))
	if string(afterData) != before {
		t.Fatalf("install should be idempotent for CLAUDE.md managed block")
	}

	msgPath := filepath.Join(tmpDir, ".git", "COMMIT_EDITMSG")
	if err := os.WriteFile(msgPath, []byte("feat: test\n\nTicket: TKT-999\n"), 0o644); err != nil {
		t.Fatalf("write commit msg failed: %v", err)
	}
	ticketPath := filepath.Join(tmpDir, ".docket", "tickets")
	if err := os.MkdirAll(ticketPath, 0o755); err != nil {
		t.Fatalf("mkdir tickets failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ticketPath, "TKT-999.md"), []byte("state: done\n"), 0o644); err != nil {
		t.Fatalf("write done ticket failed: %v", err)
	}
	hookCmd := exec.Command(preCommitHookPath(tmpDir))
	hookCmd.Dir = tmpDir
	if err := hookCmd.Run(); err == nil {
		t.Fatalf("expected hook to block done-state referenced ticket")
	}
}

func TestUpgradeCheckAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	repo = tmpDir
	defer func() { repo = oldRepo }()
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpDir, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	// Reset global flags
	installSkill = false
	installCursor = false
	installVSCode = false
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"install"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if err := os.WriteFile(preCommitHookPath(tmpDir), []byte("#!/bin/sh\necho stale\n"), 0o755); err != nil {
		t.Fatalf("write stale hook failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"upgrade", "--check"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected --check to fail for stale artifacts")
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"upgrade"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"upgrade", "--check"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected --check to pass after upgrade, got: %v", err)
	}

	cfgData, err := os.ReadFile(filepath.Join(tmpDir, ".docket", "config.yaml"))
	if err != nil {
		t.Fatalf("expected config.yaml to be managed by upgrade: %v", err)
	}
	if !strings.Contains(string(cfgData), "ticket_quality_min_ac:") {
		t.Fatalf("expected missing config key to be injected")
	}
}
