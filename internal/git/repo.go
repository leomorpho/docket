package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GetGitCommonDir returns the absolute path to the shared .git common directory.
// In a normal repo, this is the same as the .git directory.
// In a worktree, this points to the main repository's .git directory.
func GetGitCommonDir(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("getting git common dir: %w", err)
	}

	rel := strings.TrimSpace(out)
	if filepath.IsAbs(rel) {
		return rel, nil
	}

	return filepath.Abs(filepath.Join(repoRoot, rel))
}

// IsWorktree returns true if the given directory is a git worktree.
func IsWorktree(repoRoot string) (bool, error) {
	out, err := runGit(repoRoot, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}
