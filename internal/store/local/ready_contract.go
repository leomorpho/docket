package local

import (
	"fmt"
	"strings"

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
