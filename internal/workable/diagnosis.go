package workable

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

type EmptyDiagnosis struct {
	StartableStates     []string
	StartableTickets    int
	BlockedTickets      int
	ClaimedTickets      int
	CoordinationTickets int
	NonTransitionable   int
	TopBlockers         []BlockerCount
}

type BlockerCount struct {
	ID    string
	Count int
}

func DiagnoseEmpty(ctx context.Context, s *local.Store, cfg *ticket.Config) (EmptyDiagnosis, error) {
	diagnosis := EmptyDiagnosis{StartableStates: append([]string(nil), cfg.StartableStates()...)}
	if s == nil || cfg == nil || len(diagnosis.StartableStates) == 0 {
		return diagnosis, nil
	}

	f := store.Filter{States: make([]ticket.State, 0, len(diagnosis.StartableStates))}
	for _, state := range diagnosis.StartableStates {
		f.States = append(f.States, ticket.State(state))
	}

	tickets, err := s.ListTickets(ctx, f)
	if err != nil {
		return EmptyDiagnosis{}, err
	}

	blockerCounts := map[string]int{}
	for _, item := range tickets {
		full, err := s.GetTicket(ctx, item.ID)
		if err != nil {
			return EmptyDiagnosis{}, err
		}
		if full == nil {
			continue
		}
		if IsCoordinationTicket(full) {
			diagnosis.CoordinationTickets++
			continue
		}
		if !cfg.States[string(full.State)].Startable || len(cfg.StartTransitionTargets(string(full.State))) == 0 {
			diagnosis.NonTransitionable++
			continue
		}
		diagnosis.StartableTickets++

		blocked, err := blockedByClaim(s.RepoRoot, cfg, full)
		if err != nil {
			return EmptyDiagnosis{}, err
		}
		if blocked {
			diagnosis.ClaimedTickets++
			continue
		}

		unresolved, err := s.UnresolvedBlockers(ctx, full)
		if err != nil {
			return EmptyDiagnosis{}, err
		}
		if len(unresolved) == 0 {
			continue
		}
		diagnosis.BlockedTickets++
		for _, blockerID := range unresolved {
			blockerCounts[blockerID]++
		}
	}

	for id, count := range blockerCounts {
		diagnosis.TopBlockers = append(diagnosis.TopBlockers, BlockerCount{ID: id, Count: count})
	}
	sort.Slice(diagnosis.TopBlockers, func(i, j int) bool {
		if diagnosis.TopBlockers[i].Count == diagnosis.TopBlockers[j].Count {
			return diagnosis.TopBlockers[i].ID < diagnosis.TopBlockers[j].ID
		}
		return diagnosis.TopBlockers[i].Count > diagnosis.TopBlockers[j].Count
	})
	if len(diagnosis.TopBlockers) > 3 {
		diagnosis.TopBlockers = diagnosis.TopBlockers[:3]
	}
	return diagnosis, nil
}

func (d EmptyDiagnosis) Summary() string {
	startable := "none configured"
	if len(d.StartableStates) > 0 {
		startable = strings.Join(d.StartableStates, ", ")
	}
	base := fmt.Sprintf("No workable tickets found. Startable states in current config: %s.", startable)
	switch {
	case len(d.StartableStates) == 0:
		return base
	case d.StartableTickets == 0 && d.CoordinationTickets > 0:
		return fmt.Sprintf("%s Backlog warning: only coordination tickets are sitting in startable states (%d coordination tickets); no actionable leaf tickets are startable.", base, d.CoordinationTickets)
	case d.StartableTickets == 0:
		return fmt.Sprintf("%s Backlog warning: there are no actionable tickets in startable states. Check ticket state wiring and parent/blocker links.", base)
	}

	parts := []string{
		fmt.Sprintf("%d actionable tickets are in startable states", d.StartableTickets),
		fmt.Sprintf("%d blocked", d.BlockedTickets),
	}
	if d.ClaimedTickets > 0 {
		parts = append(parts, fmt.Sprintf("%d claimed", d.ClaimedTickets))
	}
	if d.CoordinationTickets > 0 {
		parts = append(parts, fmt.Sprintf("%d coordination-only hidden", d.CoordinationTickets))
	}
	if d.NonTransitionable > 0 {
		parts = append(parts, fmt.Sprintf("%d misconfigured without active transitions", d.NonTransitionable))
	}

	summary := fmt.Sprintf("%s Backlog warning: none are runnable right now; %s.", base, strings.Join(parts, ", "))
	if len(d.TopBlockers) == 0 {
		return summary + " Check blocker wiring and in-review handoff policy."
	}

	blockers := make([]string, 0, len(d.TopBlockers))
	for _, blocker := range d.TopBlockers {
		blockers = append(blockers, fmt.Sprintf("%s x%d", blocker.ID, blocker.Count))
	}
	return fmt.Sprintf("%s Top unresolved blockers: %s.", summary, strings.Join(blockers, ", "))
}

func (d EmptyDiagnosis) NoRunnableReason() string {
	summary := strings.TrimSpace(d.Summary())
	if summary == "" {
		return "no runnable tickets remain"
	}
	return "no runnable tickets remain: " + summary
}
