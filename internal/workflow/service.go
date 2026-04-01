package workflow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
)

type WorkflowManager struct {
	store   store.Backend
	vcs     vcs.Provider
	claimer claim.Manager
}

func NewManager(s store.Backend, v vcs.Provider, c claim.Manager) *WorkflowManager {
	return &WorkflowManager{
		store:   s,
		vcs:     v,
		claimer: c,
	}
}

// StartTask moves a ticket to the configured active-work state, claims it, and sets up a worktree.
// Returns the updated ticket and the worktree path where it is claimed.
func (m *WorkflowManager) StartTask(ctx context.Context, ticketID, agentID string, cfg *ticket.Config) (*ticket.Ticket, string, error) {
	t, err := m.store.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, "", fmt.Errorf("getting ticket: %w", err)
	}
	if t == nil {
		return nil, "", fmt.Errorf("ticket %s not found", ticketID)
	}

	newState, err := resolveStartState(t, cfg)
	if err != nil {
		return nil, "", err
	}
	startCmd := UpdateStateCmd{
		To:           newState,
		SetStartedAt: true,
	}
	if err := startCmd.Validate(t, cfg); err != nil {
		return nil, "", fmt.Errorf("invalid transition: %w", err)
	}

	// Handle VCS and claims.
	wtPath, wtErr := m.vcs.GetAgentWorktreeDir(ctx, t.ID)
	repoRoot, _ := m.vcs.GetRepoRoot(ctx)
	claimedPath := repoRoot

	if wtErr != nil {
		return nil, "", fmt.Errorf("ticket %s requires dedicated worktree path: %w", t.ID, wtErr)
	} else {
		currentCheckout, curErr := m.vcs.CurrentCheckoutPath(ctx)
		if curErr != nil {
			return nil, "", fmt.Errorf("resolving current checkout for %s: %w", t.ID, curErr)
		}
		currentAbs, _ := filepath.Abs(currentCheckout)
		desiredAbs, _ := filepath.Abs(wtPath)
		isPrimary, primaryErr := m.vcs.IsPrimaryCheckout(ctx)
		if primaryErr != nil {
			return nil, "", fmt.Errorf("checking current checkout type for %s: %w", t.ID, primaryErr)
		}

		if !isPrimary && currentAbs == desiredAbs {
			if err := m.claimer.Claim(ctx, t.ID, currentAbs, agentID); err != nil {
				return nil, "", fmt.Errorf("claiming ticket in existing worktree: %w", err)
			}
			claimedPath = currentAbs
		} else {
			branch := "docket/" + t.ID
			if err := m.vcs.CreateWorktree(ctx, t.ID, branch, wtPath); err != nil {
				return nil, "", fmt.Errorf("ticket %s requires dedicated worktree: %w", t.ID, err)
			}
			if err := m.claimer.Claim(ctx, t.ID, wtPath, agentID); err != nil {
				return nil, "", fmt.Errorf("claiming ticket in worktree: %w", err)
			}
			claimedPath = wtPath
		}
	}

	if claimedPath == repoRoot {
		return nil, "", fmt.Errorf("ticket %s rejected: run is not bound to a dedicated worktree", t.ID)
	}

	startCmd.Apply(t, time.Now())
	if err := m.store.UpdateTicket(ctx, t); err != nil {
		return nil, "", fmt.Errorf("updating ticket: %w", err)
	}

	return t, claimedPath, nil
}

// FinishTask moves a ticket to the configured machine-owned success state and
// releases the claim. If the ticket was in a separate worktree, it commits
// changes and merges back.
func (m *WorkflowManager) FinishTask(ctx context.Context, ticketID string, cfg *ticket.Config) (*ticket.Ticket, error) {
	return m.finishTask(ctx, ticketID, cfg, "")
}

func (m *WorkflowManager) FinishTaskWithSummary(ctx context.Context, ticketID string, cfg *ticket.Config, mergeCommitMessage string) (*ticket.Ticket, error) {
	return m.finishTask(ctx, ticketID, cfg, mergeCommitMessage)
}

func (m *WorkflowManager) finishTask(ctx context.Context, ticketID string, cfg *ticket.Config, mergeCommitMessage string) (*ticket.Ticket, error) {
	t, err := m.store.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("getting ticket: %w", err)
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}

	finishCmd, err := buildFinishStateCmd(t, cfg)
	if err != nil {
		return nil, err
	}

	// 1. Handle VCS merge-back if needed
	cl, _ := m.claimer.GetClaim(ctx, t.ID)
	repoRoot, _ := m.vcs.GetRepoRoot(ctx)
	currentCheckout, _ := m.vcs.CurrentCheckoutPath(ctx)
	currentAbs, _ := filepath.Abs(currentCheckout)
	claimedAbs := ""
	if cl != nil {
		claimedAbs, _ = filepath.Abs(cl.Worktree)
	}
	mergedFromBoundWorktree := cl != nil && cl.Worktree != "" && cl.Worktree != repoRoot && currentAbs != "" && currentAbs == claimedAbs
	if cl != nil && cl.Worktree != "" && cl.Worktree != repoRoot {
		branch := "docket/" + t.ID
		mergedFromFallbackRef := false
		recoverableCleanupFailure := false
		// Commit changes in worktree
		_ = m.vcs.CommitAll(ctx, cl.Worktree, fmt.Sprintf("Auto-commit for %s completion", t.ID))

		// Merge back
		if err := m.vcs.MergeBranch(ctx, branch, mergeCommitMessage); err != nil {
			if !isMissingMergeRefError(err) {
				return nil, fmt.Errorf("merge conflict: %w. Resolve it in %s", err, cl.Worktree)
			}
			mergeRef, refErr := docketgit.HeadSHA(cl.Worktree)
			if refErr != nil {
				return nil, fmt.Errorf("merge conflict: %w. Fallback merge from %s HEAD failed: %v", err, cl.Worktree, refErr)
			}
			if retryErr := m.vcs.MergeBranch(ctx, mergeRef, mergeCommitMessage); retryErr != nil {
				return nil, fmt.Errorf("merge conflict: %w. Resolve it in %s", retryErr, cl.Worktree)
			}
			mergedFromFallbackRef = true
		}

		// Cleanup must succeed so merged runs do not leave stale linked worktrees behind.
		if err := m.vcs.RemoveWorktree(ctx, cl.Worktree); err != nil {
			if !isRecoverableWorktreeCleanupError(err) {
				return nil, fmt.Errorf("cleanup merged worktree %s: %w", cl.Worktree, err)
			}
			recoverableCleanupFailure = true
			_ = docketgit.PruneWorktrees(repoRoot)
		}
		if !mergedFromFallbackRef {
			if err := m.vcs.DeleteBranch(ctx, branch); err != nil {
				if recoverableCleanupFailure && isRecoverableBranchDeleteError(err, cl.Worktree) {
					_ = docketgit.PruneWorktrees(repoRoot)
					if retryErr := m.vcs.DeleteBranch(ctx, branch); retryErr == nil || isRecoverableBranchDeleteError(retryErr, cl.Worktree) {
						goto postCleanup
					}
				}
				return nil, fmt.Errorf("delete merged branch %s: %w", branch, err)
			}
		} else if exists, _ := docketgit.CommitExists(repoRoot, branch); exists {
			if err := m.vcs.DeleteBranch(ctx, branch); err != nil {
				if recoverableCleanupFailure && isRecoverableBranchDeleteError(err, cl.Worktree) {
					_ = docketgit.PruneWorktrees(repoRoot)
					if retryErr := m.vcs.DeleteBranch(ctx, branch); retryErr == nil || isRecoverableBranchDeleteError(retryErr, cl.Worktree) {
						goto postCleanup
					}
				}
				return nil, fmt.Errorf("delete merged branch %s: %w", branch, err)
			}
		}
	postCleanup:
	}

	// 2. Transition state through command validation.
	targetStore := m.store
	if mergedFromBoundWorktree {
		sharedStore := local.New(repoRoot)
		mergedTicket, err := sharedStore.GetTicket(ctx, ticketID)
		if err != nil {
			return nil, fmt.Errorf("loading merged ticket from repo root: %w", err)
		}
		if mergedTicket == nil {
			return nil, fmt.Errorf("ticket %s missing from repo root after merge-back", ticketID)
		}
		t = mergedTicket
		targetStore = sharedStore
	}
	finishCmd.Apply(t, time.Now())
	if err := targetStore.UpdateTicket(ctx, t); err != nil {
		return nil, fmt.Errorf("updating ticket: %w", err)
	}

	// 3. Release claim
	_ = m.claimer.Release(ctx, t.ID)

	return t, nil
}

func buildFinishStateCmd(t *ticket.Ticket, cfg *ticket.Config) (UpdateStateCmd, error) {
	if t == nil {
		return UpdateStateCmd{}, fmt.Errorf("ticket is required")
	}
	if cfg == nil {
		return UpdateStateCmd{}, fmt.Errorf("config is required")
	}
	completedState, err := resolveFinishState(t.State, cfg)
	if err != nil {
		return UpdateStateCmd{}, err
	}
	return UpdateStateCmd{To: completedState, SetCompletedAt: true}, nil
}

func resolveFinishState(from ticket.State, cfg *ticket.Config) (ticket.State, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is required")
	}
	start := strings.TrimSpace(string(from))
	if start == "" {
		return "", fmt.Errorf("current state is required")
	}

	queue := []string{start}
	visited := map[string]bool{start: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if cfg.StateHasRole(current, "completed") {
			return ticket.State(current), nil
		}
		stateCfg, ok := cfg.States[current]
		if !ok {
			continue
		}
		for _, next := range stateCfg.Next {
			next = strings.TrimSpace(next)
			if next == "" || visited[next] {
				continue
			}
			visited[next] = true
			queue = append(queue, next)
		}
	}
	return "", fmt.Errorf("cannot transition from %s to a configured completed state", from)
}

func resolveStartState(t *ticket.Ticket, cfg *ticket.Config) (ticket.State, error) {
	if t == nil {
		return "", fmt.Errorf("ticket is required")
	}
	if cfg == nil {
		return "", fmt.Errorf("config is required")
	}
	for _, next := range cfg.StartTransitionTargets(string(t.State)) {
		startCmd := UpdateStateCmd{To: ticket.State(next)}
		if err := startCmd.Validate(t, cfg); err == nil {
			return ticket.State(next), nil
		}
	}
	// Keep start behavior aligned with workable ticket selection for multi-hop
	// workflows (for example backlog -> todo -> in-progress).
	stateCfg, ok := cfg.States[string(t.State)]
	if ok {
		for _, next := range stateCfg.Next {
			nextCfg, exists := cfg.States[next]
			if !exists || nextCfg.Terminal {
				continue
			}
			startCmd := UpdateStateCmd{To: ticket.State(next)}
			if err := startCmd.Validate(t, cfg); err == nil {
				return ticket.State(next), nil
			}
		}
	}
	return "", fmt.Errorf("cannot transition %s from %s to a configured active state", t.ID, t.State)
}

func isMissingMergeRefError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	markers := []string{
		"not something we can merge",
		"unknown revision",
		"bad revision",
		"ambiguous argument",
		"invalid object name",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func isRecoverableWorktreeCleanupError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	markers := []string{
		"permission denied",
		"operation not permitted",
		"no such file or directory",
		"does not exist",
		"not a working tree",
		"unable to access",
	}
	for _, marker := range markers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func isRecoverableBranchDeleteError(err error, worktreePath string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "checked out at") {
		return false
	}
	wt := strings.ToLower(strings.TrimSpace(worktreePath))
	if wt == "" {
		return true
	}
	return strings.Contains(msg, wt)
}
