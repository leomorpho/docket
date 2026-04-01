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
	UngroomedTickets    int
	NonTransitionable   int
	NeedsPromotion      int    // startable leaf tickets with no direct active-role next, but valid non-terminal next states
	NeedsPromotionNext  string // the next state they should be advanced to
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
	idx, err := s.BuildRelationshipIndex(ctx)
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
		if ticket.IsCoordinationTicket(full) || !localIsLeaf(idx, full.ID) {
			diagnosis.CoordinationTickets++
			continue
		}
		if len(local.RunnableContractErrors(cfg, idx, full)) > 0 {
			diagnosis.UngroomedTickets++
			continue
		}
		stateCfg, ok := cfg.States[string(full.State)]
		if !ok || !stateCfg.Startable {
			diagnosis.NonTransitionable++
			continue
		}
		// Check whether this ticket has any non-terminal next state.
		hasNonTerminalNext := false
		hasDirectActiveNext := len(cfg.StartTransitionTargets(string(full.State))) > 0
		for _, next := range stateCfg.Next {
			nextCfg, exists := cfg.States[next]
			if exists && !nextCfg.Terminal {
				hasNonTerminalNext = true
				break
			}
		}
		if !hasNonTerminalNext {
			diagnosis.NonTransitionable++
			continue
		}
		// Ticket is workable but requires a promotion hop (e.g. backlog → todo → in-progress).
		if !hasDirectActiveNext {
			diagnosis.NeedsPromotion++
			// Record the first non-terminal next state as the promotion target.
			if diagnosis.NeedsPromotionNext == "" {
				for _, next := range stateCfg.Next {
					nextCfg, exists := cfg.States[next]
					if exists && !nextCfg.Terminal {
						diagnosis.NeedsPromotionNext = next
						break
					}
				}
			}
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

func localIsLeaf(idx *local.RelationshipIndex, id string) bool {
	return idx != nil && len(idx.Children[id]) == 0
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
	case d.NeedsPromotion > 0 && d.StartableTickets == 0:
		// The common multi-hop case: backlog tickets exist but need advancing to an
		// intermediate state (e.g. todo) before they become directly workable.
		promoteTo := d.NeedsPromotionNext
		if promoteTo == "" {
			promoteTo = "<next-state>"
		}
		return fmt.Sprintf(
			"%s Action required: %d leaf ticket(s) are in startable states but require promotion to %q before work can begin. "+
				"Run: docket list --state %s --format context   then advance tickets with: docket update <TKT-ID> --state %s",
			base, d.NeedsPromotion, promoteTo, d.StartableStates[0], promoteTo,
		)
	case d.StartableTickets == 0 && d.CoordinationTickets > 0:
		return fmt.Sprintf("%s Queue warning: only coordination tickets are sitting in startable states (%d coordination tickets); no actionable leaf tickets are startable.", base, d.CoordinationTickets)
	case d.StartableTickets == 0 && d.UngroomedTickets > 0:
		return fmt.Sprintf("%s Queue warning: %d ready ticket(s) are not runnable yet because the ready contract is incomplete. Groom ready tickets with explicit context, verification, and out-of-scope boundaries before rerunning autorun.", base, d.UngroomedTickets)
	case d.StartableTickets == 0:
		return fmt.Sprintf("%s Queue warning: there are no actionable tickets in startable states. Check ticket state wiring and parent/blocker links.", base)
	}

	parts := []string{
		fmt.Sprintf("%d actionable tickets are in startable states", d.StartableTickets),
		fmt.Sprintf("%d blocked", d.BlockedTickets),
	}
	if d.ClaimedTickets > 0 {
		parts = append(parts, fmt.Sprintf("%d claimed", d.ClaimedTickets))
	}
	if d.UngroomedTickets > 0 {
		parts = append(parts, fmt.Sprintf("%d ungroomed", d.UngroomedTickets))
	}
	if d.CoordinationTickets > 0 {
		parts = append(parts, fmt.Sprintf("%d coordination-only hidden", d.CoordinationTickets))
	}
	if d.NonTransitionable > 0 {
		parts = append(parts, fmt.Sprintf("%d misconfigured without active transitions", d.NonTransitionable))
	}

	summary := fmt.Sprintf("%s Queue warning: none are runnable right now; %s.", base, strings.Join(parts, ", "))
	if len(d.TopBlockers) == 0 {
		if d.UngroomedTickets > 0 {
			return summary + " Groom ready tickets with explicit context, verification, and out-of-scope boundaries before rerunning autorun."
		}
		return summary + " Check blocker wiring and ready/validated workflow policy."
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
