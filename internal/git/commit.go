package git

import (
	"fmt"
	"strings"
)

// CommitAll stages all changes and commits them with the given message.
func CommitAll(repoRoot, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("commit message is required")
	}
	// Stage all changes
	_, err := runGit(repoRoot, "add", ".")
	if err != nil {
		return fmt.Errorf("git add . failed: %w", err)
	}

	// Check if there are any changes to commit
	out, err := runGit(repoRoot, "status", "--porcelain")
	if err == nil && strings.TrimSpace(out) == "" {
		// Nothing to commit
		return nil
	}

	// Commit
	out, err = runGit(repoRoot, "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit failed: %w\n%s", err, out)
	}
	return nil
}
