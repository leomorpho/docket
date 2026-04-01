package local

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

func requiresRunnableContract(cfg *ticket.Config, state ticket.State) bool {
	if cfg == nil {
		return false
	}
	name := strings.TrimSpace(string(state))
	if name == "" {
		return false
	}
	if stateCfg, ok := cfg.States[name]; ok && stateCfg.Startable {
		return true
	}
	return cfg.StateHasRole(name, "active")
}

func RunnableContractErrors(cfg *ticket.Config, idx *RelationshipIndex, t *ticket.Ticket) []store.ValidationError {
	if t == nil || !requiresRunnableContract(cfg, t.State) {
		return nil
	}

	return ReadyContractErrors(cfg, idx, t)
}

func ReadyContractErrors(cfg *ticket.Config, idx *RelationshipIndex, t *ticket.Ticket) []store.ValidationError {
	if t == nil {
		return nil
	}

	errs := []store.ValidationError{}
	if ticket.IsCoordinationTicket(t) {
		errs = append(errs, store.ValidationError{
			Field:   "state",
			Message: fmt.Sprintf("state %q is reserved for executable leaf tickets; coordination tickets must remain structural", t.State),
		})
	}
	if idx != nil && !isLeafTicket(idx, t.ID) {
		errs = append(errs, store.ValidationError{
			Field:   "state",
			Message: fmt.Sprintf("state %q requires a leaf ticket; parent tickets cannot enter the runnable workflow", t.State),
		})
	}

	if wordCount(t.Description) < 30 {
		errs = append(errs, store.ValidationError{
			Field:   "ready_contract.description",
			Message: "runnable tickets need at least 30 words of execution context",
		})
	}
	descLower := strings.ToLower(t.Description)
	if !strings.Contains(descLower, "likely paths:") {
		errs = append(errs, store.ValidationError{
			Field:   "ready_contract.description",
			Message: "runnable tickets must include a `Likely paths:` section",
		})
	}
	if !strings.Contains(descLower, "out of scope:") {
		errs = append(errs, store.ValidationError{
			Field:   "ready_contract.description",
			Message: "runnable tickets must include an `Out of scope:` section",
		})
	}
	hasVerifyCommands := strings.Contains(descLower, "verify commands:")
	if !hasVerifyCommands {
		errs = append(errs, store.ValidationError{
			Field:   "ready_contract.description",
			Message: "runnable tickets must include a `Verify commands:` section",
		})
	}

	if len(t.AC) < 2 {
		errs = append(errs, store.ValidationError{
			Field:   "ready_contract.ac",
			Message: "runnable tickets need at least 2 acceptance criteria",
		})
	}

	verified := hasVerifyCommands
	if !hasVerifyCommands {
		for i, ac := range t.AC {
			if strings.TrimSpace(ac.Run) != "" || len(ac.VerificationSteps) > 0 {
				verified = true
				continue
			}
			errs = append(errs, store.ValidationError{
				Field:   fmt.Sprintf("ac[%d].verification", i),
				Message: "runnable tickets require every acceptance criterion to include `run` or `verification_steps`, or the ticket description must include `Verify commands:`",
			})
		}
	}
	if !verified {
		errs = append(errs, store.ValidationError{
			Field:   "ready_contract.verification",
			Message: "runnable tickets need explicit verification attached to acceptance criteria or declared in `Verify commands:`",
		})
	}

	return errs
}

type ReadyCheckResult struct {
	TicketID string            `json:"ticket_id"`
	Ready    bool              `json:"ready"`
	State    ticket.State      `json:"state"`
	Issues   []ReadyCheckIssue `json:"issues,omitempty"`
}

type ReadyCheckIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (s *Store) PromoteReady(ctx context.Context, id string) (ReadyCheckResult, bool, error) {
	id = s.normalizeTicketLookupID(id)
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return ReadyCheckResult{}, false, err
	}
	if t == nil {
		return ReadyCheckResult{}, false, fmt.Errorf("ticket %s not found", id)
	}

	if t.State == ticket.State("ready") {
		report, reportErr := s.CheckReady(ctx, id)
		return report, false, reportErr
	}
	if t.State != ticket.State("draft") {
		return ReadyCheckResult{}, false, fmt.Errorf("ticket %s must be in draft to promote with ready --promote", t.ID)
	}

	report, err := s.CheckReady(ctx, id)
	if err != nil {
		return ReadyCheckResult{}, false, err
	}
	if !report.Ready {
		return report, false, fmt.Errorf("ready contract failed for %s", report.TicketID)
	}

	t.State = ticket.State("ready")
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(ctx, t); err != nil {
		return ReadyCheckResult{}, false, err
	}
	if err := s.SyncIndex(ctx); err != nil {
		return ReadyCheckResult{}, false, err
	}

	report, err = s.CheckReady(ctx, id)
	if err != nil {
		return ReadyCheckResult{}, false, err
	}
	return report, true, nil
}

func (s *Store) CheckReady(ctx context.Context, id string) (ReadyCheckResult, error) {
	id = s.normalizeTicketLookupID(id)
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return ReadyCheckResult{}, err
	}
	if t == nil {
		return ReadyCheckResult{}, fmt.Errorf("ticket %s not found", id)
	}

	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		return ReadyCheckResult{}, err
	}

	cfg, cfgErr := ticket.LoadConfig(s.RepoRoot)
	if cfgErr != nil {
		cfg = ticket.DefaultConfig()
	}

	rawIssues := ReadyContractErrors(cfg, idx, t)
	issues := make([]ReadyCheckIssue, 0, len(rawIssues))
	for _, issue := range rawIssues {
		issues = append(issues, ReadyCheckIssue{
			Field:   issue.Field,
			Message: issue.Message,
		})
	}
	return ReadyCheckResult{
		TicketID: t.ID,
		Ready:    len(issues) == 0,
		State:    t.State,
		Issues:   issues,
	}, nil
}
