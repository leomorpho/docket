package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

type WorkflowManager struct {
	store   store.Backend
	vcs     VCSProvider
	claimer ClaimManager
}

func NewManager(s store.Backend, v VCSProvider, c ClaimManager) *WorkflowManager {
	return &WorkflowManager{
		store:   s,
		vcs:     v,
		claimer: c,
	}
}

// StartTask moves a ticket to 'in-progress', claims it, and sets up a worktree.
// Returns the updated ticket and the worktree path where it is claimed.
func (m *WorkflowManager) StartTask(ctx context.Context, ticketID, agentID string, cfg *ticket.Config) (*ticket.Ticket, string, error) {
	t, err := m.store.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, "", fmt.Errorf("getting ticket: %w", err)
	}
	if t == nil {
		return nil, "", fmt.Errorf("ticket %s not found", ticketID)
	}

	newState := ticket.State("in-progress")
	if err := ticket.ValidateTransition(cfg, t.State, newState); err != nil {
		return nil, "", fmt.Errorf("invalid transition: %w", err)
	}

	t.State = newState
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if t.StartedAt.IsZero() {
		t.StartedAt = t.UpdatedAt
	}

	// Handle VCS and Claims
	wtPath, wtErr := m.vcs.GetAgentWorktreeDir(ctx, t.ID)
	repoRoot, _ := m.vcs.GetRepoRoot(ctx)
	claimedPath := repoRoot
	
	if wtErr == nil {
		branch := "docket/" + t.ID
		if err := m.vcs.CreateWorktree(ctx, t.ID, branch, wtPath); err == nil {
			_ = m.claimer.Claim(ctx, t.ID, wtPath, agentID)
			claimedPath = wtPath
		} else {
			// Fallback to repo root
			_ = m.claimer.Claim(ctx, t.ID, repoRoot, agentID)
		}
	} else {
		_ = m.claimer.Claim(ctx, t.ID, repoRoot, agentID)
	}

	if err := m.store.UpdateTicket(ctx, t); err != nil {
		return nil, "", fmt.Errorf("updating ticket: %w", err)
	}

	return t, claimedPath, nil
}

// FinishTask moves a ticket to 'done' (or 'in-review') and releases the claim.
// If the ticket was in a separate worktree, it commits changes and merges back.
func (m *WorkflowManager) FinishTask(ctx context.Context, ticketID string, cfg *ticket.Config) (*ticket.Ticket, error) {
	t, err := m.store.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("getting ticket: %w", err)
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}

	// 1. Handle VCS merge-back if needed
	cl, _ := m.claimer.GetClaim(ctx, t.ID)
	repoRoot, _ := m.vcs.GetRepoRoot(ctx)
	if cl != nil && cl.Worktree != "" && cl.Worktree != repoRoot {
		branch := "docket/" + t.ID
		// Commit changes in worktree
		_ = m.vcs.CommitAll(ctx, cl.Worktree, fmt.Sprintf("Auto-commit for %s completion", t.ID))
		
		// Merge back
		if err := m.vcs.MergeBranch(ctx, branch); err != nil {
			return nil, fmt.Errorf("merge conflict: %w. Resolve it in %s", err, cl.Worktree)
		}
		
		// Cleanup
		_ = m.vcs.RemoveWorktree(ctx, cl.Worktree)
		_ = m.vcs.DeleteBranch(ctx, branch)
	}

	// 2. Transition state
	newState := ticket.State("done")
	if !cfg.IsValidState(string(newState)) {
		newState = ticket.State("completed")
	}

	if err := ticket.ValidateTransition(cfg, t.State, newState); err != nil {
		reviewState := ticket.State("in-review")
		if err2 := ticket.ValidateTransition(cfg, t.State, reviewState); err2 == nil {
			newState = reviewState
		} else {
			return nil, fmt.Errorf("cannot transition %s from %s to done or in-review", ticketID, t.State)
		}
	}

	t.State = newState
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if t.CompletedAt.IsZero() && newState == "done" {
		t.CompletedAt = t.UpdatedAt
	}

	if err := m.store.UpdateTicket(ctx, t); err != nil {
		return nil, fmt.Errorf("updating ticket: %w", err)
	}

	// 3. Release claim
	_ = m.claimer.Release(ctx, t.ID)

	return t, nil
}
