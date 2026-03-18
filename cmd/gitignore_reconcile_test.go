package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoLocalGitignoreLinesCoverLegacyAndCanonicalPaths(t *testing.T) {
	lines := repoLocalGitignoreLines()
	required := []string{
		"# docket",
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
