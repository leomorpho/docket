package workflow

import (
	"fmt"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

type TicketCommand interface {
	Validate(t *ticket.Ticket, cfg *ticket.Config) error
	Apply(t *ticket.Ticket, now time.Time)
}

type UpdateStateCmd struct {
	To             ticket.State
	SetStartedAt   bool
	SetCompletedAt bool
}

func (c UpdateStateCmd) Validate(t *ticket.Ticket, cfg *ticket.Config) error {
	if t == nil {
		return fmt.Errorf("ticket is required")
	}
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if !cfg.IsValidState(string(c.To)) {
		return fmt.Errorf("invalid target state: %q", c.To)
	}
	return ticket.ValidateTransition(cfg, t.State, c.To)
}

func (c UpdateStateCmd) Apply(t *ticket.Ticket, now time.Time) {
	t.State = c.To
	t.UpdatedAt = now.UTC().Truncate(time.Second)
	if c.SetStartedAt && t.StartedAt.IsZero() {
		t.StartedAt = t.UpdatedAt
	}
	if c.SetCompletedAt && t.CompletedAt.IsZero() && c.To == "done" {
		t.CompletedAt = t.UpdatedAt
	}
}
