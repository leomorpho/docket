package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexConfigPath(t *testing.T) {
	repo := t.TempDir()
	if got := codexConfigPath(repo); got != filepath.Join(repo, "doombox.json") {
		t.Fatalf("unexpected codex config path: %s", got)
	}
}

func TestCodexInstallMergesConfigWithoutClobbering(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "doombox.json"), []byte(`{"theme":"custom","mcp":"legacy"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}

	adapter := newCodexAdapter()
	if err := adapter.Install(context.Background(), InstallInput{RepoRoot: repo}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(repo, "doombox.json"))
	if err != nil {
		t.Fatalf("read doombox.json failed: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse doombox.json failed: %v", err)
	}
	if cfg["mcp"] != "docket" {
		t.Fatalf("expected mcp=docket, got %#v", cfg["mcp"])
	}
	if cfg["theme"] != "custom" {
		t.Fatalf("expected unrelated key preserved, got %#v", cfg)
	}

	agentsRaw, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(agentsRaw)), "docket") {
		t.Fatalf("expected AGENTS.md to include docket guidance, got: %s", string(agentsRaw))
	}
}

func TestCodexDoctorProvidesRemediationWhenMissing(t *testing.T) {
	repo := t.TempDir()
	adapter := newCodexAdapter()

	report, err := adapter.Doctor(context.Background(), repo)
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if len(report.Checks) < 3 {
		t.Fatalf("expected doctor checks, got %#v", report.Checks)
	}

	fails := 0
	for _, chk := range report.Checks {
		if chk.OK {
			continue
		}
		fails++
		if !strings.Contains(chk.Detail, "docket bootstrap --adapter codex") && chk.Name != "hooks" {
			t.Fatalf("expected remediation detail in %s, got %q", chk.Name, chk.Detail)
		}
	}
	if fails == 0 {
		t.Fatalf("expected failing checks in empty repo, got %#v", report.Checks)
	}
}

func TestCodexBootstrapThenStatusReady(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write commit-msg failed: %v", err)
	}

	adapter := newCodexAdapter()
	if err := adapter.Bootstrap(context.Background(), BootstrapInput{RepoRoot: repo}); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	status, err := adapter.Status(context.Background(), repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !status.Ready {
		t.Fatalf("expected codex status ready, got %+v", status)
	}
}
