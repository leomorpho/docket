package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateWorktree creates a new git worktree for a ticket.
func CreateWorktree(repoRoot, ticketID, branch, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Check if branch already exists
	_, err := runGit(repoRoot, "rev-parse", "--verify", branch)
	args := []string{"worktree", "add", "-b", branch, path}
	if err == nil {
		// Branch exists, just add worktree pointing to it
		args = []string{"worktree", "add", path, branch}
	}

	out, err := runGit(repoRoot, args...)
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w\n%s", err, out)
	}
	return nil
}

// RemoveWorktree removes a git worktree and prunes it.
func RemoveWorktree(repoRoot, path string) error {
	out, err := runGit(repoRoot, "worktree", "remove", "--force", path)
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w\n%s", err, out)
	}
	_, _ = runGit(repoRoot, "worktree", "prune")
	return nil
}

// GetAgentWorktreeDir returns the default directory for agent worktrees.
func GetAgentWorktreeDir(ticketID string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	// Use a subdirectory for docket worktrees
	// We include a hash of the current working directory to avoid collisions between different projects
	cwd, _ := os.Getwd()
	projectID := filepath.Base(cwd)
	if len(projectID) > 16 {
		projectID = projectID[:16]
	}
	
	return filepath.Join(cacheDir, "docket", "worktrees", projectID, ticketID), nil
}
