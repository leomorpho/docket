package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func enforceRunnableTicketContract(ctx context.Context, s *local.Store, cfg *ticket.Config, t *ticket.Ticket) error {
	if s == nil || cfg == nil || t == nil {
		return nil
	}
	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		return err
	}
	errs := runnableContractIssues(cfg, idx, t)
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, 0, len(errs))
	for _, issue := range errs {
		parts = append(parts, issue.Message)
	}
	return fmt.Errorf("ticket %s cannot enter %s: %s", t.ID, t.State, strings.Join(parts, "; "))
}

func runnableContractIssues(cfg *ticket.Config, idx *local.RelationshipIndex, t *ticket.Ticket) []store.ValidationError {
	if t == nil {
		return nil
	}
	return local.RunnableContractErrors(cfg, idx, t)
}
