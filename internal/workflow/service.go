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
func (m *WorkflowManager) StartTask(ctx context.Context, ticketID, agentID string, cfg *ticket.Config) (*ticket.Ticket, error) {
	t, err := m.store.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("getting ticket: %w", err)
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}

	newState := ticket.State("in-progress")
	if err := ticket.ValidateTransition(cfg, t.State, newState); err != nil {
		return nil, fmt.Errorf("invalid transition: %w", err)
	}

	t.State = newState
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if t.StartedAt.IsZero() {
		t.StartedAt = t.UpdatedAt
	}

	// Handle VCS and Claims
	wtPath, wtErr := m.vcs.GetAgentWorktreeDir(ctx, t.ID)
	repoRoot, _ := m.vcs.GetRepoRoot(ctx)
	
	if wtErr == nil {
		branch := "docket/" + t.ID
		if err := m.vcs.CreateWorktree(ctx, t.ID, branch, wtPath); err == nil {
			_ = m.claimer.Claim(ctx, t.ID, wtPath, agentID)
		} else {
			// Fallback to repo root
			_ = m.claimer.Claim(ctx, t.ID, repoRoot, agentID)
		}
	} else {
		_ = m.claimer.Claim(ctx, t.ID, repoRoot, agentID)
	}

	if err := m.store.UpdateTicket(ctx, t); err != nil {
		return nil, fmt.Errorf("updating ticket: %w", err)
	}

	return t, nil
}

// FinishTask moves a ticket to 'done' (or 'in-review') and releases the claim.
func (m *WorkflowManager) FinishTask(ctx context.Context, ticketID string, cfg *ticket.Config) (*ticket.Ticket, error) {
	t, err := m.store.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("getting ticket: %w", err)
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}

	// For now we'll support direct to 'done' or 'in-review'
	newState := ticket.State("done")
	if !cfg.IsValidState(string(newState)) {
		newState = ticket.State("completed") // Fallback or check config
	}

	// If 'in-review' exists and is preferred by config, we could use it.
	// For simplicity, we use 'done'.
	if err := ticket.ValidateTransition(cfg, t.State, newState); err != nil {
		// Try 'in-review' as alternative
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

	// Release claim
	_ = m.claimer.Release(ctx, t.ID)

	return t, nil
}
