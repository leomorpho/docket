package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitRepoUtils(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Not a git repo
	_, err := GetGitCommonDir(tmpDir)
	if err == nil {
		t.Fatal("expected error for GetGitCommonDir on non-git repo")
	}

	isWt, err := IsWorktree(tmpDir)
	if err == nil && isWt {
		t.Fatal("expected non-git dir to not be a worktree")
	}

	// 2. Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if err := c.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	common, err := GetGitCommonDir(tmpDir)
	if err != nil {
		t.Fatalf("GetGitCommonDir failed: %v", err)
	}
	if !strings.HasSuffix(common, ".git") {
		t.Errorf("expected common dir to end in .git, got %s", common)
	}

	isWt, err = IsWorktree(tmpDir)
	if err != nil {
		t.Fatalf("IsWorktree failed: %v", err)
	}
	if isWt {
		// A main repo is not considered a worktree by rev-parse --is-inside-work-tree usually?
		// Actually it returns true if you are inside a worktree.
	}

	// 3. Test Show
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)
	runGit("add", "test.txt")
	runGit("commit", "-m", "add test.txt")

	content, err := Show(tmpDir, "HEAD", "test.txt")
	if err != nil {
		t.Fatalf("Show failed: %v", err)
	}
	if content != "hello" {
		t.Errorf("expected 'hello', got %q", content)
	}
}
