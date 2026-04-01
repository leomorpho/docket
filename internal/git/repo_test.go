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

func TestGetGitCommonDirAndSharedRepoRootUseDotGitMetadataWithoutGitBinary(t *testing.T) {
	tmpDir := t.TempDir()

	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "seed")

	worktreePath := filepath.Join(tmpDir, "worktrees", "TKT-001")
	runGit("worktree", "add", "-b", "docket/TKT-001", worktreePath)
	wantRoot := normalizeRepoPath(tmpDir)

	t.Setenv("PATH", filepath.Join(tmpDir, "missing-bin"))

	commonMain, err := GetGitCommonDir(tmpDir)
	if err != nil {
		t.Fatalf("GetGitCommonDir(main) failed without git binary: %v", err)
	}
	if got, want := SharedRepoRoot(tmpDir), wantRoot; got != want {
		t.Fatalf("SharedRepoRoot(main) = %q, want %q", got, want)
	}

	commonWorktree, err := GetGitCommonDir(worktreePath)
	if err != nil {
		t.Fatalf("GetGitCommonDir(worktree) failed without git binary: %v", err)
	}
	if got, want := SharedRepoRoot(worktreePath), wantRoot; got != want {
		t.Fatalf("SharedRepoRoot(worktree) = %q, want %q", got, want)
	}
	if commonMain != commonWorktree {
		t.Fatalf("expected shared common dir for main and worktree, got %q vs %q", commonMain, commonWorktree)
	}
	if !strings.HasSuffix(commonMain, ".git") {
		t.Fatalf("expected common dir to end in .git, got %q", commonMain)
	}
}
