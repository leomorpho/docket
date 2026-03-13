package git

import (
	"os"
	"os/exec"
	"path/filepath"
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
	wtDir, err := GetAgentWorktreeDir(ticketID)
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
