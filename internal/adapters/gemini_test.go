package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGeminiInstallIdempotentAndCreatesArtifacts(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	adapter := newGeminiAdapter()
	if err := adapter.Install(context.Background(), InstallInput{RepoRoot: repo}); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if err := adapter.Install(context.Background(), InstallInput{RepoRoot: repo}); err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(repo, "GEMINI.md"),
		filepath.Join(home, ".gemini", "skills", "docket", "SKILL.md"),
		filepath.Join(home, ".gemini", "settings.json"),
		filepath.Join(repo, ".cursor", "mcp.json"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected installed artifact %s: %v", p, err)
		}
	}
}

func TestGeminiInstallPreservesExistingConfig(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir failed: %v", err)
	}
	seed := `{"theme":"solarized","mcp":{"other":{"command":"x"}}}`
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write settings failed: %v", err)
	}

	adapter := newGeminiAdapter()
	if err := adapter.Install(context.Background(), InstallInput{RepoRoot: repo}); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse settings failed: %v", err)
	}
	if payload["theme"] != "solarized" {
		t.Fatalf("expected unrelated setting preserved, got %#v", payload)
	}
	mcp, _ := payload["mcp"].(map[string]any)
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("expected existing mcp entry preserved, got %#v", mcp)
	}
	if _, ok := mcp["docket"]; !ok {
		t.Fatalf("expected docket mcp entry added, got %#v", mcp)
	}
}

func TestGeminiInstallMissingHomePathErrorHandling(t *testing.T) {
	repo := t.TempDir()
	badHomeFile := filepath.Join(t.TempDir(), "home-file")
	if err := os.WriteFile(badHomeFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write bad home file failed: %v", err)
	}
	t.Setenv("HOME", badHomeFile)

	adapter := newGeminiAdapter()
	if err := adapter.Install(context.Background(), InstallInput{RepoRoot: repo}); err == nil {
		t.Fatal("expected install to fail when HOME resolves to non-directory path")
	}
}

func TestGeminiBootstrapStatusReady(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(repo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write pre-commit failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write commit-msg failed: %v", err)
	}

	adapter := newGeminiAdapter()
	if err := adapter.Bootstrap(context.Background(), BootstrapInput{RepoRoot: repo}); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	status, err := adapter.Status(context.Background(), repo)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !status.Ready {
		t.Fatalf("expected gemini status ready, got %+v", status)
	}
}
