package workable

import (
	"context"
	"strings"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func Tickets(ctx context.Context, s *local.Store, cfg *ticket.Config, f store.Filter) ([]*ticket.Ticket, error) {
	startableStates := cfg.StartableStates()
	if len(startableStates) == 0 {
		return nil, nil
	}

	startableSet := make(map[ticket.State]struct{}, len(startableStates))
	for _, state := range startableStates {
		startableSet[ticket.State(state)] = struct{}{}
	}
	if len(f.States) == 0 {
		f.States = make([]ticket.State, 0, len(startableStates))
		for state := range startableSet {
			f.States = append(f.States, state)
		}
	} else {
		filteredStates := make([]ticket.State, 0, len(f.States))
		for _, state := range f.States {
			if _, ok := startableSet[state]; ok {
				filteredStates = append(filteredStates, state)
			}
		}
		f.States = filteredStates
	}
	f.OnlyUnblocked = true

	tickets, err := s.ListTickets(ctx, f)
	if err != nil {
		return nil, err
	}

	out := make([]*ticket.Ticket, 0, len(tickets))
	for _, t := range tickets {
		full, err := s.GetTicket(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		if full == nil || !IsTicket(cfg, full) {
			continue
		}
		blocked, err := blockedByClaim(s.RepoRoot, cfg, full)
		if err != nil {
			return nil, err
		}
		if blocked {
			continue
		}
		out = append(out, full)
	}
	return out, nil
}

func IsTicket(cfg *ticket.Config, t *ticket.Ticket) bool {
	if cfg == nil || t == nil {
		return false
	}
	stateCfg, ok := cfg.States[string(t.State)]
	if !ok || !stateCfg.Startable {
		return false
	}
	if IsCoordinationTicket(t) {
		return false
	}
	return len(cfg.StartTransitionTargets(string(t.State))) > 0
}

func IsCoordinationTicket(t *ticket.Ticket) bool {
	for _, l := range t.Labels {
		label := strings.ToLower(strings.TrimSpace(l))
		if label == "epic" || label == "program" || label == "topo:coordination" {
			return true
		}
	}
	title := strings.TrimSpace(t.Title)
	return strings.HasPrefix(title, "[Epic]") ||
		strings.HasPrefix(title, "Epic:") ||
		strings.HasPrefix(title, "Program:")
}

func blockedByClaim(repoRoot string, cfg *ticket.Config, t *ticket.Ticket) (bool, error) {
	cl, err := claim.GetClaim(repoRoot, t.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not a git repository") {
			return false, nil
		}
		return false, err
	}
	if cl == nil {
		return false, nil
	}
	if isStaleStartableClaim(cfg, t) {
		if err := claim.Release(repoRoot, t.ID); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func isStaleStartableClaim(cfg *ticket.Config, t *ticket.Ticket) bool {
	if cfg == nil || t == nil {
		return false
	}
	stateCfg, ok := cfg.States[string(t.State)]
	if !ok || !stateCfg.Startable {
		return false
	}
	return len(cfg.StartTransitionTargets(string(t.State))) > 0
}
