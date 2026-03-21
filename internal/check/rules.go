package check

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
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

type Rule func(ctx context.Context, backend store.Backend, cfg *ticket.Config, t *ticket.Ticket, now time.Time) []Finding

func RuleR001(ctx context.Context, backend store.Backend, cfg *ticket.Config, t *ticket.Ticket, now time.Time) []Finding {
	if cfg == nil || !cfg.StateHasRole(string(t.State), "active") {
		return nil
	}
	if now.Sub(t.UpdatedAt) <= 7*24*time.Hour {
		return nil
	}
	days := int(now.Sub(t.UpdatedAt).Hours() / 24)
	return []Finding{{TicketID: t.ID, Rule: "R001", Severity: SeverityWarn, Message: fmt.Sprintf("No activity for %d days (state: %s)", days, t.State), AutoFix: false}}
}

func RuleR006(ctx context.Context, backend store.Backend, cfg *ticket.Config, t *ticket.Ticket, now time.Time) []Finding {
	var out []Finding
	for _, blockerID := range t.BlockedBy {
		blocker, err := backend.GetTicket(ctx, blockerID)
		if err != nil || blocker == nil {
			continue
		}
		if !cfg.BlocksDependents(blocker.State) {
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

func RuleR008(ctx context.Context, backend store.Backend, cfg *ticket.Config, t *ticket.Ticket, now time.Time) []Finding {
	errs, err := backend.Validate(ctx, t.ID)
	if err != nil {
		return []Finding{{TicketID: t.ID, Rule: "R008", Severity: SeverityError, Message: err.Error(), AutoFix: false}}
	}
	if len(errs) == 0 {
		return nil
	}
	return []Finding{{TicketID: t.ID, Rule: "R008", Severity: SeverityError, Message: errs[0].Field + ": " + errs[0].Message, AutoFix: false}}
}

func RuleR009(ctx context.Context, backend store.Backend, cfg *ticket.Config, t *ticket.Ticket, now time.Time) []Finding {
	issues, err := DescendantClosureIssues(ctx, backend, cfg, t)
	if err != nil {
		return []Finding{{TicketID: t.ID, Rule: "R009", Severity: SeverityError, Message: err.Error(), AutoFix: false}}
	}
	if len(issues) == 0 {
		return nil
	}
	findings := make([]Finding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, Finding{
			TicketID: t.ID,
			Rule:     "R009",
			Severity: SeverityError,
			Message:  issue,
			AutoFix:  false,
		})
	}
	return findings
}

func DescendantClosureIssues(ctx context.Context, backend store.Backend, cfg *ticket.Config, root *ticket.Ticket) ([]string, error) {
	if root == nil {
		return nil, nil
	}
	if cfg == nil {
		cfg = ticket.DefaultConfig()
	}

	all, err := backend.ListTickets(ctx, store.Filter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	byID := make(map[string]*ticket.Ticket, len(all))
	children := make(map[string][]*ticket.Ticket)
	for _, candidate := range all {
		byID[candidate.ID] = candidate
		if candidate.Parent != "" {
			children[candidate.Parent] = append(children[candidate.Parent], candidate)
		}
	}
	for parentID := range children {
		sort.Slice(children[parentID], func(i, j int) bool {
			return children[parentID][i].ID < children[parentID][j].ID
		})
	}

	var descendants []*ticket.Ticket
	var walk func(string)
	walk = func(parentID string) {
		for _, child := range children[parentID] {
			descendants = append(descendants, child)
			walk(child.ID)
		}
	}
	walk(root.ID)
	if len(descendants) == 0 {
		return nil, nil
	}

	issues := make([]string, 0, len(descendants))
	for _, desc := range descendants {
		if !stateCountsAsClosed(cfg, desc.State) {
			issues = append(issues, fmt.Sprintf("descendant %s is still %s", desc.ID, desc.State))
			continue
		}

		errs, err := backend.Validate(ctx, desc.ID)
		if err != nil {
			return nil, err
		}
		if len(errs) > 0 {
			issues = append(issues, fmt.Sprintf("descendant %s failed validation: %s: %s", desc.ID, errs[0].Field, errs[0].Message))
			continue
		}

		for _, blockerID := range desc.BlockedBy {
			blocker, ok := byID[blockerID]
			if !ok || blocker == nil {
				continue
			}
			if cfg.BlocksDependents(blocker.State) {
				issues = append(issues, fmt.Sprintf("descendant %s is still blocked by %s (%s)", desc.ID, blockerID, blocker.State))
				break
			}
		}
	}

	return issues, nil
}

func stateCountsAsClosed(cfg *ticket.Config, state ticket.State) bool {
	return cfg.StateHasRole(string(state), "completed") || cfg.StateHasRole(string(state), "archived")
}
