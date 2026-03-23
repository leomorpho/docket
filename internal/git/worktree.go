package git

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateWorktree creates a new git worktree for a ticket.
func CreateWorktree(repoRoot, ticketID, branch, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if _, err := runGit(repoRoot, "worktree", "prune"); err != nil {
		return fmt.Errorf("git worktree prune failed: %w", err)
	}

	registeredBranch, registered, err := registeredWorktreeBranch(repoRoot, path)
	if err != nil {
		return err
	}
	if registered {
		if registeredBranch == "" || registeredBranch == branch {
			return restoreTrackedDeletions(path)
		}
		return fmt.Errorf("worktree path %s is already registered to branch %s", path, registeredBranch)
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("worktree path %s exists and is not a directory", path)
		}
		managedPath, managedErr := GetAgentWorktreeDir(repoRoot, ticketID)
		if managedErr != nil {
			return fmt.Errorf("resolve managed worktree path for %s: %w", ticketID, managedErr)
		}
		if !samePath(path, managedPath) {
			return fmt.Errorf("orphaned worktree path %s is outside managed cache path %s", path, managedPath)
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove orphaned worktree path %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
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

	_, err = runGit(repoRoot, "rev-parse", "--verify", branch)
	args := []string{"worktree", "add", "-b", branch, path}
	if err == nil {
		// Branch exists, just add worktree pointing to it
		args = []string{"worktree", "add", path, branch}
	}

	out, err := runGit(repoRoot, args...)
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w\n%s", err, out)
	}
	return restoreTrackedDeletions(path)
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

func registeredWorktreeBranch(repoRoot, path string) (string, bool, error) {
	out, err := runGit(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("git worktree list failed: %w", err)
	}

	desired, err := filepath.Abs(path)
	if err != nil {
		return "", false, err
	}

	var currentPath string
	var currentBranch string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			if currentPath != "" {
				if samePath(currentPath, desired) {
					return currentBranch, true, nil
				}
			}
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			currentBranch = ""
		case strings.HasPrefix(line, "branch "):
			currentBranch = strings.TrimPrefix(strings.TrimSpace(line), "branch refs/heads/")
		case line == "":
			if currentPath != "" && samePath(currentPath, desired) {
				return currentBranch, true, nil
			}
			currentPath = ""
			currentBranch = ""
		}
	}
	if currentPath != "" && samePath(currentPath, desired) {
		return currentBranch, true, nil
	}

	return "", false, nil
}

func samePath(a, b string) bool {
	aAbs, errA := canonicalPath(a)
	bAbs, errB := canonicalPath(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return aAbs == bAbs
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if os.IsNotExist(err) {
		parent := filepath.Dir(abs)
		resolvedParent, parentErr := filepath.EvalSymlinks(parent)
		if parentErr == nil {
			return filepath.Join(resolvedParent, filepath.Base(abs)), nil
		}
	}
	return abs, nil
}

func restoreTrackedDeletions(worktreePath string) error {
	out, err := runGit(worktreePath, "ls-files", "--deleted", "-z")
	if err != nil {
		return fmt.Errorf("list deleted tracked files in %s: %w", worktreePath, err)
	}
	trimmed := strings.TrimSuffix(out, "\x00")
	if trimmed == "" {
		return nil
	}
	paths := strings.Split(trimmed, "\x00")
	args := []string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}
	args = append(args, paths...)
	if _, err := runGit(worktreePath, args...); err != nil {
		return fmt.Errorf("restore deleted tracked files in %s: %w", worktreePath, err)
	}
	return nil
}
