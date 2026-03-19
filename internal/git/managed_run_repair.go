package git

import (
	"fmt"
	"strings"
)

type ManagedRunRepairResult struct {
	Repaired     bool
	Method       string
	SourceRef    string
	CommitSHAs   []string
	TargetBranch string
}

func RepairManagedBranchFromCurrent(repoRoot, worktreePath, managedBranch, ticketID, sinceRFC3339 string) (ManagedRunRepairResult, error) {
	result := ManagedRunRepairResult{TargetBranch: strings.TrimSpace(managedBranch)}
	if strings.TrimSpace(worktreePath) == "" {
		return result, fmt.Errorf("worktree path is required")
	}
	if strings.TrimSpace(managedBranch) == "" {
		return result, fmt.Errorf("managed branch is required")
	}
	if strings.TrimSpace(ticketID) == "" {
		return result, fmt.Errorf("ticket ID is required")
	}

	sourceRef, err := CurrentBranch(repoRoot)
	if err != nil {
		return result, fmt.Errorf("resolve current branch: %w", err)
	}
	if sourceRef == "" {
		sourceRef = "HEAD"
	}
	result.SourceRef = sourceRef

	if sourceRef == managedBranch {
		return result, nil
	}
	if ok, err := HasTicketTrailerSince(repoRoot, managedBranch, ticketID, sinceRFC3339); err == nil && ok {
		return result, nil
	}

	commitSHAs, err := TicketCommitSHAsSince(repoRoot, "HEAD", ticketID, sinceRFC3339)
	if err != nil {
		return result, err
	}
	if len(commitSHAs) == 0 {
		return result, nil
	}
	result.CommitSHAs = append([]string{}, commitSHAs...)

	clean, err := IsClean(worktreePath)
	if err != nil {
		return result, fmt.Errorf("check managed worktree cleanliness: %w", err)
	}
	if !clean {
		return result, fmt.Errorf("managed worktree %s has uncommitted changes; cannot auto-repair branch drift safely", worktreePath)
	}

	headSHA, err := HeadSHA(repoRoot)
	if err != nil {
		return result, err
	}
	canFF, err := IsAncestor(repoRoot, managedBranch, headSHA)
	if err != nil {
		return result, err
	}
	if canFF {
		if _, err := runGit(worktreePath, "merge", "--ff-only", headSHA); err != nil {
			return result, fmt.Errorf("fast-forward managed branch %s to %s: %w", managedBranch, headSHA, err)
		}
		result.Repaired = true
		result.Method = "fast-forward"
		return result, nil
	}

	for _, sha := range commitSHAs {
		if _, err := runGit(worktreePath, "cherry-pick", "-x", sha); err != nil {
			_, _ = runGit(worktreePath, "cherry-pick", "--abort")
			return result, fmt.Errorf("cherry-pick %s into managed branch %s: %w", sha, managedBranch, err)
		}
	}
	result.Repaired = true
	result.Method = "cherry-pick"
	return result, nil
}

func IsClean(repoRoot string) (bool, error) {
	out, err := runGit(repoRoot, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

func IsAncestor(repoRoot, ancestorRef, descendantRef string) (bool, error) {
	if strings.TrimSpace(ancestorRef) == "" || strings.TrimSpace(descendantRef) == "" {
		return false, fmt.Errorf("refs are required")
	}
	if _, err := runGit(repoRoot, "merge-base", "--is-ancestor", ancestorRef, descendantRef); err != nil {
		if _, revErr := runGit(repoRoot, "rev-parse", "--verify", ancestorRef); revErr != nil {
			return false, revErr
		}
		if _, revErr := runGit(repoRoot, "rev-parse", "--verify", descendantRef); revErr != nil {
			return false, revErr
		}
		return false, nil
	}
	return true, nil
}
