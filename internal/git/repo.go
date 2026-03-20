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

// GetRepoRoot returns the absolute path to the root of the git repository.
func GetRepoRoot(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("getting git repo root: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// SharedRepoRoot returns the canonical shared repository root for Docket-managed
// metadata. In a normal checkout this is the repo root; in a git worktree it is
// the main checkout that owns the shared git common dir.
func SharedRepoRoot(repoRoot string) string {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return ""
	}
	if commonDir, err := GetGitCommonDir(repoRoot); err == nil && strings.TrimSpace(commonDir) != "" {
		if absRoot, absErr := filepath.Abs(filepath.Dir(commonDir)); absErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(absRoot); resolveErr == nil {
				return resolved
			}
			return absRoot
		}
		return filepath.Dir(commonDir)
	}
	if absRoot, err := filepath.Abs(repoRoot); err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(absRoot); resolveErr == nil {
			return resolved
		}
		return absRoot
	}
	return repoRoot
}

// IsWorktree returns true if the given directory is a git worktree.
func IsWorktree(repoRoot string) (bool, error) {
	out, err := runGit(repoRoot, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

// Show returns the content of a file at a specific git ref.
func Show(repoRoot, ref, path string) (string, error) {
	return runGit(repoRoot, "show", ref+":"+path)
}

func CurrentBranch(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func HeadSHA(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func CommitExists(repoRoot, ref string) (bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false, fmt.Errorf("commit ref is required")
	}
	if _, err := runGit(repoRoot, "rev-parse", "--verify", ref+"^{commit}"); err != nil {
		return false, nil
	}
	return true, nil
}
