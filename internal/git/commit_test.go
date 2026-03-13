package git

import (
	"os"
	"os/exec"
	"testing"
)

func TestCommitAll(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		_ = c.Run()
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	// Create a file
	os.WriteFile(tmpDir+"/test.txt", []byte("hello"), 0644)

	// CommitAll
	if err := CommitAll(tmpDir, "Initial commit"); err != nil {
		t.Fatalf("CommitAll failed: %v", err)
	}

	// Verify commit exists
	c := exec.Command("git", "-C", tmpDir, "log", "-1", "--format=%s")
	out, err := c.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if string(out) != "Initial commit\n" {
		t.Errorf("expected 'Initial commit', got %q", string(out))
	}

	// 2. Empty message
	if err := CommitAll(tmpDir, ""); err == nil {
		t.Error("expected error for empty commit message")
	}
}
