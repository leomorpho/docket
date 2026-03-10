package check

import (
	"context"
	"time"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

type Checker struct {
	Backend store.Backend
	Now     func() time.Time
	Rules   []Rule
}

func NewChecker(backend store.Backend) *Checker {
	return &Checker{
		Backend: backend,
		Now:     func() time.Time { return time.Now().UTC() },
		Rules:   []Rule{RuleR001, RuleR006, RuleR008},
	}
}

func (c *Checker) Run(ctx context.Context, tickets []*ticket.Ticket, fix bool) ([]Finding, error) {
	now := c.Now()
	var findings []Finding

	for _, t := range tickets {
		if fix {
			if fixed, err := c.fixResolvedBlockers(ctx, t); err != nil {
				return nil, err
			} else if fixed {
				// Re-load the updated ticket so remaining rules run on current state.
				updated, err := c.Backend.GetTicket(ctx, t.ID)
				if err != nil {
					return nil, err
				}
				if updated != nil {
					t = updated
				}
			}
		}

		for _, r := range c.Rules {
			findings = append(findings, r(ctx, c.Backend, t, now)...)
		}
	}

	return findings, nil
}

func (c *Checker) fixResolvedBlockers(ctx context.Context, t *ticket.Ticket) (bool, error) {
	if len(t.BlockedBy) == 0 {
		return false, nil
	}

	resolved := map[string]struct{}{}
	for _, blockerID := range t.BlockedBy {
		blocker, err := c.Backend.GetTicket(ctx, blockerID)
		if err != nil {
			return false, err
		}
		if blocker == nil {
			continue
		}
		if blocker.State == ticket.StateDone || blocker.State == ticket.StateArchived {
			resolved[blockerID] = struct{}{}
		}
	}
	if len(resolved) == 0 {
		return false, nil
	}

	filtered := make([]string, 0, len(t.BlockedBy))
	for _, id := range t.BlockedBy {
		if _, ok := resolved[id]; !ok {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == len(t.BlockedBy) {
		return false, nil
	}
	copyT := *t
	copyT.BlockedBy = filtered
	if err := c.Backend.UpdateTicket(ctx, &copyT); err != nil {
		return false, err
	}
	return true, nil
}
