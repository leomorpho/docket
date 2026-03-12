package git

import (
	"fmt"
	"strings"
)

// MergeBranch merges the given branch into the current HEAD of repoRoot.
func MergeBranch(repoRoot, branch string) error {
	out, err := runGit(repoRoot, "merge", "--no-ff", "-m", fmt.Sprintf("Merge ticket branch %s", branch), branch)
	if err != nil {
		// If merge failed, we might want to abort it to keep the main repo clean
		_, _ = runGit(repoRoot, "merge", "--abort")
		return fmt.Errorf("merge failed: %w\n%s", err, out)
	}
	return nil
}

// GetDefaultBranch returns the name of the default branch (main or master).
func GetDefaultBranch(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		parts := strings.Split(strings.TrimSpace(out), "/")
		return parts[len(parts)-1], nil
	}
	
	// Fallback to local branches
	for _, b := range []string{"main", "master"} {
		_, err := runGit(repoRoot, "rev-parse", "--verify", b)
		if err == nil {
			return b, nil
		}
	}
	return "main", nil
}

// DeleteBranch deletes a local branch.
func DeleteBranch(repoRoot, branch string) error {
	out, err := runGit(repoRoot, "branch", "-D", branch)
	if err != nil {
		return fmt.Errorf("branch deletion failed: %w\n%s", err, out)
	}
	return nil
}
