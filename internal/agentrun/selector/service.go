package selector

import (
	"context"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
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
	return workablepkg.Tickets(ctx, s, cfg, f)
}

func isWorkableTicket(cfg *ticket.Config, t *ticket.Ticket) bool {
	return workablepkg.IsTicket(cfg, t)
}
