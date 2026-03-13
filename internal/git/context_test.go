package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlameFile(t *testing.T) {
	repo := initRepo(t)
	mustWriteGit(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {\n\tprintln(\"x\")\n}\n")
	runGitCtx(t, repo, "add", "main.go")
	runGitCtx(t, repo, "commit", "-m", "feat: main", "-m", "Ticket: TKT-001")

	entries, err := BlameFile(repo, "main.go", 0, 0)
	if err != nil {
		t.Fatalf("BlameFile failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}

	ranged, err := BlameFile(repo, "main.go", 4, 4)
	if err != nil {
		t.Fatalf("BlameFile ranged failed: %v", err)
	}
	if len(ranged) == 0 {
		t.Fatal("expected ranged entries")
	}
	for _, e := range ranged {
		if e.Line != 4 {
			t.Fatalf("line = %d, want 4", e.Line)
		}
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	runGitCtx(t, d, "init")
	runGitCtx(t, d, "config", "user.email", "test@example.com")
	runGitCtx(t, d, "config", "user.name", "Test User")
	return d
}

func mustWriteGit(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func runGitCtx(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func TestBlameFileError(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := BlameFile(tmpDir, "missing.txt", 0, 0)
	if err == nil {
		t.Error("expected error for missing file")
	}
}
