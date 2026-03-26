package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoLocalGitignoreLinesCoverLegacyAndCanonicalPaths(t *testing.T) {
	lines := repoLocalGitignoreLines()
	required := []string{
		"# docket",
		".docket/checkpoints/",
		".docket/index.db",
		".docket/local/",
		".docket/locks.json",
		".docket/runtime/",
		".docket/semantic/",
		".docket/tickets/*/sessions/",
	}

	for _, want := range required {
		if !containsLine(lines, want) {
			t.Fatalf("missing required gitignore line %q from %v", want, lines)
		}
	}
}

func TestEnsureLocalArtifactsGitignoredUntracksLegacyLocalFiles(t *testing.T) {
	repoRoot := t.TempDir()
	runGitSession(t, repoRoot, "init")
	runGitSession(t, repoRoot, "config", "user.email", "test@example.com")
	runGitSession(t, repoRoot, "config", "user.name", "Test User")

	checkpointPath := filepath.Join(repoRoot, ".docket", "checkpoints", "TKT-001-20260312T205245Z.json")
	if err := os.MkdirAll(filepath.Dir(checkpointPath), 0o755); err != nil {
		t.Fatalf("mkdir checkpoints: %v", err)
	}
	if err := os.WriteFile(checkpointPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	runGitSession(t, repoRoot, "add", ".")
	runGitSession(t, repoRoot, "commit", "-m", "seed tracked checkpoint")

	if err := ensureLocalArtifactsGitignored(repoRoot); err != nil {
		t.Fatalf("ensureLocalArtifactsGitignored: %v", err)
	}

	gitignoreData, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignoreData), ".docket/checkpoints/\n") {
		t.Fatalf("expected checkpoints ignore rule, got:\n%s", string(gitignoreData))
	}

	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "--", ".docket/checkpoints")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files failed: %v\n%s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("expected checkpoint files to be untracked, got:\n%s", string(out))
	}
}

func TestEnsureLocalArtifactsGitignoredSkipsNonGitWorktree(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir pseudo git dir: %v", err)
	}
	if err := ensureLocalArtifactsGitignored(repoRoot); err != nil {
		t.Fatalf("ensureLocalArtifactsGitignored on pseudo repo: %v", err)
	}
}

func TestReconcileGitignoreFileIsIdempotentAndDeduplicatesManagedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	initial := strings.Join([]string{
		"node_modules/",
		"# docket",
		".docket/runtime/",
		".docket/runtime/",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial gitignore: %v", err)
	}

	changed, err := reconcileGitignoreFile(path, repoLocalGitignoreLines())
	if err != nil {
		t.Fatalf("reconcile first pass: %v", err)
	}
	if !changed {
		t.Fatal("expected first reconcile to change file")
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gitignore after first pass: %v", err)
	}
	content := string(first)
	if strings.Count(content, ".docket/runtime/") != 1 {
		t.Fatalf("expected duplicate managed lines to be removed, got:\n%s", content)
	}
	if !strings.Contains(content, ".docket/local/\n") {
		t.Fatalf("expected canonical local ignore line, got:\n%s", content)
	}
	if !strings.Contains(content, "node_modules/") {
		t.Fatalf("expected non-managed lines preserved, got:\n%s", content)
	}

	changed, err = reconcileGitignoreFile(path, repoLocalGitignoreLines())
	if err != nil {
		t.Fatalf("reconcile second pass: %v", err)
	}
	if changed {
		t.Fatal("expected second reconcile to be a no-op")
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
