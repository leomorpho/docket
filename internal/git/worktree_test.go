package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktree(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	runGit := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmpDir}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "test")

	// Create initial commit
	os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root"), 0644)
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	agentID := "agent-1"
	ticketID := "TKT-001"
	branch := "worktree-" + ticketID
	wtPath := filepath.Join(tmpDir, "worktrees", agentID)

	// 1. Create worktree
	err := CreateWorktree(tmpDir, ticketID, branch, wtPath)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree path %s does not exist", wtPath)
	}

	// 2. Resolve worktree dir
	wtDir, err := GetAgentWorktreeDir(tmpDir, ticketID)
	if err != nil {
		t.Fatalf("GetAgentWorktreeDir failed: %v", err)
	}
	if wtDir == "" {
		t.Error("expected non-empty worktree dir")
	}

	// 3. Verify it's a worktree
	isWt, err := IsWorktree(wtPath)
	if err != nil {
		t.Fatalf("IsWorktree failed on wtPath: %v", err)
	}
	if !isWt {
		t.Fatal("expected wtPath to be identified as worktree")
	}

	// 4. Remove worktree
	if err := RemoveWorktree(tmpDir, wtPath); err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatal("expected worktree path to be gone after removal")
	}
}

func TestCreateWorktree_ReusesExistingRegisteredPath(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	wtPath := filepath.Join(tmpDir, "worktrees", "TKT-201")
	branch := "docket/TKT-201"
	if err := CreateWorktree(tmpDir, "TKT-201", branch, wtPath); err != nil {
		t.Fatalf("initial CreateWorktree failed: %v", err)
	}

	if err := CreateWorktree(tmpDir, "TKT-201", branch, wtPath); err != nil {
		t.Fatalf("expected CreateWorktree to reuse registered path, got: %v", err)
	}
}

func TestCreateWorktree_ReusedPathRestoresTrackedDeletions(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "framework", "web", "components", "gen"), 0o755); err != nil {
		t.Fatalf("mkdir gen failed: %v", err)
	}
	genFile := filepath.Join(tmpDir, "framework", "web", "components", "gen", "card_templ.go")
	if err := os.WriteFile(genFile, []byte("package gen\n"), 0o644); err != nil {
		t.Fatalf("write gen file failed: %v", err)
	}
	runGit("add", "README", "framework/web/components/gen/card_templ.go")
	runGit("commit", "-m", "initial")

	wtPath := filepath.Join(tmpDir, "worktrees", "TKT-201")
	branch := "docket/TKT-201"
	if err := CreateWorktree(tmpDir, "TKT-201", branch, wtPath); err != nil {
		t.Fatalf("initial CreateWorktree failed: %v", err)
	}

	worktreeGenFile := filepath.Join(wtPath, "framework", "web", "components", "gen", "card_templ.go")
	if err := os.Remove(worktreeGenFile); err != nil {
		t.Fatalf("remove worktree gen file failed: %v", err)
	}

	if err := CreateWorktree(tmpDir, "TKT-201", branch, wtPath); err != nil {
		t.Fatalf("expected CreateWorktree to restore tracked deletions on reuse, got: %v", err)
	}
	if _, err := os.Stat(worktreeGenFile); err != nil {
		t.Fatalf("expected tracked deletion to be restored, got err=%v", err)
	}
}

func TestCreateWorktree_ReplacesOrphanedDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cacheHome := t.TempDir()
	t.Setenv("HOME", cacheHome)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cacheHome, ".cache"))

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
	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	wtPath, err := GetAgentWorktreeDir(tmpDir, "TKT-201")
	if err != nil {
		t.Fatalf("GetAgentWorktreeDir failed: %v", err)
	}
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir orphan path failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write orphan file failed: %v", err)
	}

	if err := CreateWorktree(tmpDir, "TKT-201", "docket/TKT-201", wtPath); err != nil {
		t.Fatalf("expected CreateWorktree to replace orphaned directory, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected orphan contents to be removed, got err=%v", err)
	}
}

func TestCreateWorktree_RejectsOrphanedDirectoryOutsideManagedCache(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(tmpDir, "README"), []byte("root"), 0o644); err != nil {
		t.Fatalf("write seed failed: %v", err)
	}
	runGit("add", "README")
	runGit("commit", "-m", "initial")

	wtPath := filepath.Join(tmpDir, "manual-worktrees", "TKT-201")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir orphan path failed: %v", err)
	}
	stalePath := filepath.Join(wtPath, "stale.txt")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write orphan file failed: %v", err)
	}

	err := CreateWorktree(tmpDir, "TKT-201", "docket/TKT-201", wtPath)
	if err == nil {
		t.Fatal("expected CreateWorktree to reject orphaned directory outside managed cache")
	}
	if !strings.Contains(err.Error(), "outside managed cache path") {
		t.Fatalf("expected managed cache rejection, got: %v", err)
	}
	if _, statErr := os.Stat(stalePath); statErr != nil {
		t.Fatalf("expected orphan contents preserved, got err=%v", statErr)
	}
}
