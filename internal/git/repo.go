package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var gitCommonDirCache sync.Map

func normalizeRepoPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func resolveCommonDirFromDotGit(repoRoot string) (string, bool, error) {
	repoRoot = normalizeRepoPath(repoRoot)
	if repoRoot == "" {
		return "", false, nil
	}
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false, nil
	}
	if info.IsDir() {
		return normalizeRepoPath(gitPath), true, nil
	}

	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false, err
	}
	line := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(strings.ToLower(line), "gitdir:") {
		return "", false, nil
	}
	gitDir := strings.TrimSpace(line[len("gitdir:"):])
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	gitDir = normalizeRepoPath(gitDir)
	commonDir := gitDir
	if rawCommon, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		commonDir = strings.TrimSpace(string(rawCommon))
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(gitDir, commonDir)
		}
	} else if strings.Contains(filepath.ToSlash(gitDir), "/.git/worktrees/") {
		commonDir = filepath.Dir(filepath.Dir(gitDir))
	}
	return normalizeRepoPath(commonDir), true, nil
}

// GetGitCommonDir returns the absolute path to the shared .git common directory.
// In a normal repo, this is the same as the .git directory.
// In a worktree, this points to the main repository's .git directory.
func GetGitCommonDir(repoRoot string) (string, error) {
	repoRoot = normalizeRepoPath(repoRoot)
	if cached, ok := gitCommonDirCache.Load(repoRoot); ok {
		return cached.(string), nil
	}
	if commonDir, ok, err := resolveCommonDirFromDotGit(repoRoot); err != nil {
		return "", fmt.Errorf("reading .git metadata: %w", err)
	} else if ok {
		gitCommonDirCache.Store(repoRoot, commonDir)
		return commonDir, nil
	}

	out, err := runGit(repoRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("getting git common dir: %w", err)
	}

	rel := strings.TrimSpace(out)
	if filepath.IsAbs(rel) {
		rel = normalizeRepoPath(rel)
		gitCommonDirCache.Store(repoRoot, rel)
		return rel, nil
	}

	commonDir := normalizeRepoPath(filepath.Join(repoRoot, rel))
	gitCommonDirCache.Store(repoRoot, commonDir)
	return commonDir, nil
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
	repoRoot = normalizeRepoPath(repoRoot)
	if repoRoot == "" {
		return ""
	}
	if commonDir, err := GetGitCommonDir(repoRoot); err == nil && strings.TrimSpace(commonDir) != "" {
		return normalizeRepoPath(filepath.Dir(commonDir))
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
