package check

import (
	"context"
	"fmt"
	"time"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

type Severity string

const (
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Finding struct {
	TicketID string   `json:"ticket_id"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	AutoFix  bool     `json:"auto_fix"`
}

type Rule func(ctx context.Context, backend store.Backend, t *ticket.Ticket, now time.Time) []Finding

func RuleR001(ctx context.Context, backend store.Backend, t *ticket.Ticket, now time.Time) []Finding {
	if t.State != ticket.StateInProgress {
		return nil
	}
	if now.Sub(t.UpdatedAt) <= 7*24*time.Hour {
		return nil
	}
	days := int(now.Sub(t.UpdatedAt).Hours() / 24)
	return []Finding{{TicketID: t.ID, Rule: "R001", Severity: SeverityWarn, Message: fmt.Sprintf("No activity for %d days (state: %s)", days, t.State), AutoFix: false}}
}

func RuleR006(ctx context.Context, backend store.Backend, t *ticket.Ticket, now time.Time) []Finding {
	var out []Finding
	for _, blockerID := range t.BlockedBy {
		blocker, err := backend.GetTicket(ctx, blockerID)
		if err != nil || blocker == nil {
			continue
		}
		if blocker.State == ticket.StateDone || blocker.State == ticket.StateArchived {
			out = append(out, Finding{
				TicketID: t.ID,
				Rule:     "R006",
				Severity: SeverityError,
				Message:  fmt.Sprintf("blocked_by %s is %s", blockerID, blocker.State),
				AutoFix:  true,
			})
		}
	}
	return out
}

func RuleR008(ctx context.Context, backend store.Backend, t *ticket.Ticket, now time.Time) []Finding {
	errs, err := backend.Validate(ctx, t.ID)
	if err != nil {
		return []Finding{{TicketID: t.ID, Rule: "R008", Severity: SeverityError, Message: err.Error(), AutoFix: false}}
	}
	if len(errs) == 0 {
		return nil
	}
	return []Finding{{TicketID: t.ID, Rule: "R008", Severity: SeverityError, Message: errs[0].Field + ": " + errs[0].Message, AutoFix: false}}
}
