package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAdapterInstallMergesMCPConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".cursor"), 0o755); err != nil {
		t.Fatalf("mkdir .cursor failed: %v", err)
	}
	seed := `{"servers":{"other":{"command":"x"}},"theme":"light"}`
	if err := os.WriteFile(filepath.Join(repo, ".cursor", "mcp.json"), []byte(seed), 0o644); err != nil {
		t.Fatalf("write mcp.json failed: %v", err)
	}

	adapter := newClaudeCodeAdapter()
	if err := adapter.Install(context.Background(), InstallInput{RepoRoot: repo}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(repo, ".cursor", "mcp.json"))
	if err != nil {
		t.Fatalf("read mcp.json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse mcp.json failed: %v", err)
	}
	servers, _ := payload["servers"].(map[string]any)
	if _, ok := servers["docket"]; !ok {
		t.Fatalf("expected docket server merged into mcp config, got %#v", payload)
	}
	if _, ok := servers["other"]; !ok {
		t.Fatalf("expected unrelated servers preserved, got %#v", payload)
	}
	if payload["theme"] != "light" {
		t.Fatalf("expected unrelated top-level keys preserved, got %#v", payload)
	}
}

func TestClaudeAdapterDoctorRemediationWhenMissing(t *testing.T) {
	repo := t.TempDir()
	adapter := newClaudeCodeAdapter()
	report, err := adapter.Doctor(context.Background(), repo)
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected checks in doctor report")
	}
	failCount := 0
	for _, chk := range report.Checks {
		if chk.OK {
			continue
		}
		failCount++
		if !strings.Contains(chk.Detail, "docket bootstrap --adapter claude-code") && chk.Name != "hooks" {
			t.Fatalf("expected remediation details for %s, got %q", chk.Name, chk.Detail)
		}
	}
	if failCount == 0 {
		t.Fatalf("expected missing setup failures, got %#v", report.Checks)
	}
}

func TestClaudeAdapterBootstrapStatusReady(t *testing.T) {
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

	adapter := newClaudeCodeAdapter()
	if err := adapter.Bootstrap(context.Background(), BootstrapInput{RepoRoot: repo}); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	status, err := adapter.Status(context.Background(), repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !status.Ready {
		t.Fatalf("expected claude status ready, got %+v", status)
	}
}
