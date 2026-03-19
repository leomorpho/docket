package git

import (
	"crypto/sha1"
	"encoding/hex"
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
	_, headErr := runGit(repoRoot, "rev-parse", "--verify", "HEAD")
	if headErr != nil {
		out, err := runGit(repoRoot, "worktree", "add", "--orphan", branch, path)
		if err != nil {
			return fmt.Errorf("git worktree add failed: %w\n%s", err, out)
		}
		return nil
	}

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

// GetAgentWorktreeDir returns the default directory for ticket worktrees.
func GetAgentWorktreeDir(repoRoot, ticketID string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	repoAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	projectID := filepath.Base(repoAbs)
	if len(projectID) > 16 {
		projectID = projectID[:16]
	}
	sum := sha1.Sum([]byte(repoAbs))
	repoKey := hex.EncodeToString(sum[:4])

	return filepath.Join(cacheDir, "docket", "worktrees", projectID+"-"+repoKey, ticketID), nil
}
