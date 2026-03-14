package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
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
	startCmd := UpdateStateCmd{
		To:           newState,
		SetStartedAt: true,
	}
	if err := startCmd.Validate(t, cfg); err != nil {
		return nil, "", fmt.Errorf("invalid transition: %w", err)
	}

	isAgentManaged := strings.HasPrefix(agentID, "agent:")

	// Handle VCS and claims.
	wtPath, wtErr := m.vcs.GetAgentWorktreeDir(ctx, t.ID)
	repoRoot, _ := m.vcs.GetRepoRoot(ctx)
	claimedPath := repoRoot

	if wtErr != nil {
		if isAgentManaged {
			return nil, "", fmt.Errorf("agent-managed run requires dedicated worktree path for %s: %w", t.ID, wtErr)
		}
		_ = m.claimer.Claim(ctx, t.ID, repoRoot, agentID)
	} else {
		branch := "docket/" + t.ID
		if err := m.vcs.CreateWorktree(ctx, t.ID, branch, wtPath); err != nil {
			if isAgentManaged {
				return nil, "", fmt.Errorf("agent-managed run requires dedicated worktree for %s: %w", t.ID, err)
			}
			_ = m.claimer.Claim(ctx, t.ID, repoRoot, agentID)
		} else {
			if err := m.claimer.Claim(ctx, t.ID, wtPath, agentID); err != nil {
				return nil, "", fmt.Errorf("claiming ticket in worktree: %w", err)
			}
			claimedPath = wtPath
		}
	}

	if isAgentManaged && claimedPath == repoRoot {
		return nil, "", fmt.Errorf("agent-managed run for %s rejected: run is not bound to a dedicated worktree", t.ID)
	}

	startCmd.Apply(t, time.Now())
	if err := m.store.UpdateTicket(ctx, t); err != nil {
		return nil, "", fmt.Errorf("updating ticket: %w", err)
	}

	return t, claimedPath, nil
}

// FinishTask moves a ticket to 'in-review' and releases the claim.
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

		// Cleanup must succeed so merged runs do not leave stale linked worktrees behind.
		if err := m.vcs.RemoveWorktree(ctx, cl.Worktree); err != nil {
			return nil, fmt.Errorf("cleanup merged worktree %s: %w", cl.Worktree, err)
		}
		if err := m.vcs.DeleteBranch(ctx, branch); err != nil {
			return nil, fmt.Errorf("delete merged branch %s: %w", branch, err)
		}
	}

	// 2. Transition state through command validation.
	finishCmd, err := buildFinishStateCmd(t, cfg)
	if err != nil {
		return nil, err
	}
	finishCmd.Apply(t, time.Now())

	if err := m.store.UpdateTicket(ctx, t); err != nil {
		return nil, fmt.Errorf("updating ticket: %w", err)
	}

	// 3. Release claim
	_ = m.claimer.Release(ctx, t.ID)

	return t, nil
}

func buildFinishStateCmd(t *ticket.Ticket, cfg *ticket.Config) (UpdateStateCmd, error) {
	reviewCmd := UpdateStateCmd{To: "in-review"}
	if err := reviewCmd.Validate(t, cfg); err == nil {
		return reviewCmd, nil
	}
	return UpdateStateCmd{}, fmt.Errorf("cannot transition %s from %s to in-review", t.ID, t.State)
}
