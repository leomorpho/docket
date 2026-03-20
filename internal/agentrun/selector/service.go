package selector

import (
	"context"
	"strings"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

type Dependencies struct {
	Store      *local.Store
	LoadConfig func(repoRoot string) (*ticket.Config, error)
}

type Service struct {
	store      *local.Store
	loadConfig func(repoRoot string) (*ticket.Config, error)
}

func New(deps Dependencies) *Service {
	loadConfig := deps.LoadConfig
	if loadConfig == nil {
		loadConfig = ticket.LoadConfig
	}
	return &Service{
		store:      deps.Store,
		loadConfig: loadConfig,
	}
}

func (s *Service) Next(ctx context.Context) (agentrun.Selection, error) {
	if s.store == nil {
		return agentrun.Selection{}, nil
	}
	if err := s.store.SyncIndex(ctx); err != nil {
		return agentrun.Selection{}, err
	}
	cfg, err := s.loadConfig(s.store.RepoRoot)
	if err != nil {
		return agentrun.Selection{}, err
	}
	tickets, err := workableTickets(ctx, s.store, cfg, store.Filter{})
	if err != nil {
		return agentrun.Selection{}, err
	}
	if len(tickets) == 0 {
		return agentrun.Selection{Found: false, Reason: "no runnable tickets remain"}, nil
	}
	return agentrun.Selection{
		Found:    true,
		TicketID: tickets[0].ID,
		Reason:   "next runnable ticket",
	}, nil
}

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
		cl, err := lookupClaimIfAvailable(s.RepoRoot, full.ID)
		if err != nil {
			return nil, err
		}
		if cl != nil {
			continue
		}
		out = append(out, full)
	}
	return out, nil
}

func lookupClaimIfAvailable(repoRoot, ticketID string) (*claim.ClaimMetadata, error) {
	cl, err := claim.GetClaim(repoRoot, ticketID)
	if err != nil && strings.Contains(err.Error(), "not a git repository") {
		return nil, nil
	}
	return cl, err
}

func isWorkableTicket(cfg *ticket.Config, t *ticket.Ticket) bool {
	if cfg == nil || t == nil {
		return false
	}
	stateCfg, ok := cfg.States[string(t.State)]
	if !ok || !stateCfg.Startable {
		return false
	}
	if len(cfg.StartTransitionTargets(string(t.State))) == 0 {
		return false
	}
	for _, label := range t.Labels {
		if label == "epic" {
			return false
		}
	}
	return true
}
