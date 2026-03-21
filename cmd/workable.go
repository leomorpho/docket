package cmd

import (
	"context"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
)

func workableTickets(ctx context.Context, s *local.Store, cfg *ticket.Config, f store.Filter) ([]*ticket.Ticket, error) {
	return workablepkg.Tickets(ctx, s, cfg, f)
}

func isWorkableTicket(cfg *ticket.Config, t *ticket.Ticket) bool {
	return workablepkg.IsTicket(cfg, t)
}

func isCoordinationTicket(t *ticket.Ticket) bool {
	return workablepkg.IsCoordinationTicket(t)
}
