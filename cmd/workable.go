package cmd

import (
	"context"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func workableTickets(ctx context.Context, s *local.Store, cfg *ticket.Config, f store.Filter) ([]*ticket.Ticket, error) {
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
		if full == nil || !isWorkableTicket(cfg, full) {
			continue
		}
		out = append(out, full)
	}
	return out, nil
}

func isWorkableTicket(cfg *ticket.Config, t *ticket.Ticket) bool {
	if cfg == nil || t == nil {
		return false
	}
	stateCfg, ok := cfg.States[string(t.State)]
	if !ok || !stateCfg.Startable {
		return false
	}
	if isEpicTicket(t) {
		return false
	}
	return len(cfg.StartTransitionTargets(string(t.State))) > 0
}

func isEpicTicket(t *ticket.Ticket) bool {
	for _, l := range t.Labels {
		if strings.EqualFold(strings.TrimSpace(l), "epic") {
			return true
		}
	}
	return strings.HasPrefix(strings.TrimSpace(t.Title), "[Epic]")
}
