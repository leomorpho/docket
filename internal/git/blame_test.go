package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlameLineAndCommitTicket(t *testing.T) {
	repo := setupGitRepo(t)

	file := filepath.Join(repo, "main.go")
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	runGitCmd(t, repo, "add", "main.go")
	runGitCmd(t, repo, "commit", "-m", "feat: initial", "-m", "Ticket: TKT-001")

	res, err := BlameLine(repo, "main.go", 4)
	if err != nil {
		t.Fatalf("BlameLine failed: %v", err)
	}
	if res.SHA == "" {
		t.Fatal("expected non-empty SHA")
	}
	if res.Author == "" {
		t.Fatal("expected non-empty Author")
	}
	if res.Summary == "" {
		t.Fatal("expected non-empty Summary")
	}

	ticketID, err := CommitTicket(repo, res.SHA)
	if err != nil {
		t.Fatalf("CommitTicket failed: %v", err)
	}
	if ticketID != "TKT-001" {
		t.Fatalf("ticket id = %q, want %q", ticketID, "TKT-001")
	}
}

func TestCommitTicket_NoTrailer(t *testing.T) {
	repo := setupGitRepo(t)

	file := filepath.Join(repo, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	runGitCmd(t, repo, "add", "main.go")
	runGitCmd(t, repo, "commit", "-m", "chore: no trailer")

	sha := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	ticketID, err := CommitTicket(repo, sha)
	if err != nil {
		t.Fatalf("CommitTicket failed: %v", err)
	}
	if ticketID != "" {
		t.Fatalf("ticket id = %q, want empty", ticketID)
	}
}

func TestBlameLine_Errors(t *testing.T) {
	repo := setupGitRepo(t)

	if _, err := BlameLine(repo, "missing.go", 1); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := BlameLine(repo, "missing.go", 0); err == nil {
		t.Fatal("expected error for invalid line")
	}
}

func setupGitRepo(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	runGitCmd(t, repo, "init")
	runGitCmd(t, repo, "config", "user.email", "test@example.com")
	runGitCmd(t, repo, "config", "user.name", "Test User")
	return repo
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}
