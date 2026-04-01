package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStarterScaffoldLayoutMatchesGoldenContract(t *testing.T) {
	tmpRepo := t.TempDir()

	got := starterScaffoldLayout(tmpRepo)
	want := []string{
		filepath.Join(".docket", "install.json"),
		filepath.Join(".git", "hooks", "commit-msg"),
		filepath.Join(".git", "hooks", "post-merge"),
		filepath.Join(".git", "hooks", "pre-commit"),
		".gitignore",
		"CLAUDE.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("starter scaffold layout mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBootstrapWritesStarterScaffoldLayout(t *testing.T) {
	tmpRepo := t.TempDir()
	t.Setenv("DOCKET_HOME", filepath.Join(t.TempDir(), "docket-home"))
	docketHome = ""
	repo = tmpRepo
	format = "json"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir .git/hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}

	if _, err := executeBootstrap(context.Background(), tmpRepo, "codex", bootstrapDeps{}); err != nil {
		t.Fatalf("executeBootstrap failed: %v", err)
	}

	if err := validateStarterScaffoldLayout(tmpRepo); err != nil {
		t.Fatalf("validateStarterScaffoldLayout failed: %v", err)
	}

	manifestData, err := os.ReadFile(installManifestPath(tmpRepo))
	if err != nil {
		t.Fatalf("read install manifest failed: %v", err)
	}
	var manifest installManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("parse install manifest failed: %v", err)
	}
	if !reflect.DeepEqual(manifest.ManagedArtifacts, starterScaffoldManagedArtifacts(tmpRepo)) {
		t.Fatalf("managed artifacts mismatch\n got: %q\nwant: %q", manifest.ManagedArtifacts, starterScaffoldManagedArtifacts(tmpRepo))
	}

	gitignoreData, err := os.ReadFile(filepath.Join(tmpRepo, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore failed: %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".docket/local/") {
		t.Fatalf(".gitignore should ignore canonical local root, got:\n%s", string(gitignoreData))
	}
}

func TestStarterScaffoldManagedArtifactsIncludeTrackedInstallManifest(t *testing.T) {
	tmpRepo := t.TempDir()

	managed := starterScaffoldManagedArtifacts(tmpRepo)
	installManifest := installManifestPath(tmpRepo)
	if !containsArtifactPath(managed, installManifest) {
		t.Fatalf("managed artifacts should include tracked install manifest %q, got %q", installManifest, managed)
	}
}

func containsArtifactPath(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
