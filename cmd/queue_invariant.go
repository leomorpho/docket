package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
	"github.com/spf13/cobra"
)

type queueInvariantError struct {
	Diagnosis workablepkg.EmptyDiagnosis
}

func (e *queueInvariantError) Error() string {
	summary := strings.TrimSpace(e.Diagnosis.Summary())
	if summary == "" {
		summary = "No workable tickets found."
	}
	return fmt.Sprintf("%s Queue invariant violated: at least one unblocked startable leaf ticket is required. Fix blockers or run `docket queue heal`; use --allow-empty-startable-leaf only for emergency overrides.", summary)
}

func enforceStartableLeafInvariant(ctx context.Context, s *local.Store, cfg *ticket.Config, allow bool) error {
	if allow {
		return nil
	}
	enforce, err := shouldEnforceQueueInvariant(ctx, s, cfg)
	if err != nil {
		return err
	}
	if !enforce {
		return nil
	}
	workable, err := workableTickets(ctx, s, cfg, store.Filter{})
	if err != nil {
		return fmt.Errorf("checking workable queue invariant: %w", err)
	}
	if len(workable) > 0 {
		return nil
	}
	diagnosis, err := workablepkg.DiagnoseEmpty(ctx, s, cfg)
	if err != nil {
		return fmt.Errorf("diagnosing empty workable queue: %w", err)
	}
	return &queueInvariantError{Diagnosis: diagnosis}
}

func workableStartableLeafCount(ctx context.Context, s *local.Store, cfg *ticket.Config) (int, error) {
	if err := s.SyncIndex(ctx); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0, nil
		}
		return 0, fmt.Errorf("syncing ticket index: %w", err)
	}
	workable, err := workableTickets(ctx, s, cfg, store.Filter{})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0, nil
		}
		return 0, fmt.Errorf("checking workable queue size: %w", err)
	}
	return len(workable), nil
}

func enforceStartableLeafInvariantDelta(ctx context.Context, s *local.Store, cfg *ticket.Config, allow bool, beforeCount int) error {
	if allow || beforeCount <= 0 {
		return nil
	}
	enforce, err := shouldEnforceQueueInvariant(ctx, s, cfg)
	if err != nil {
		return err
	}
	if !enforce {
		return nil
	}
	afterCount, err := workableStartableLeafCount(ctx, s, cfg)
	if err != nil {
		return err
	}
	if afterCount > 0 {
		return nil
	}
	diagnosis, err := workablepkg.DiagnoseEmpty(ctx, s, cfg)
	if err != nil {
		return fmt.Errorf("diagnosing empty workable queue: %w", err)
	}
	return &queueInvariantError{Diagnosis: diagnosis}
}

func shouldEnforceQueueInvariant(ctx context.Context, s *local.Store, cfg *ticket.Config) (bool, error) {
	if s == nil || cfg == nil {
		return false, nil
	}
	if err := s.SyncIndex(ctx); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return false, nil
		}
		return false, fmt.Errorf("syncing ticket index: %w", err)
	}
	filter := store.Filter{States: make([]ticket.State, 0, len(cfg.OpenStates()))}
	for _, st := range cfg.OpenStates() {
		filter.States = append(filter.States, ticket.State(st))
	}
	tickets, err := s.ListTickets(ctx, filter)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return false, nil
		}
		return false, fmt.Errorf("listing tickets for queue invariant policy: %w", err)
	}
	for _, t := range tickets {
		full, err := s.GetTicket(ctx, t.ID)
		if err != nil {
			return false, fmt.Errorf("loading ticket %s for queue invariant policy: %w", t.ID, err)
		}
		if full == nil {
			continue
		}
		for _, label := range full.Labels {
			trimmed := strings.ToLower(strings.TrimSpace(label))
			if trimmed == "topo:leaf" || trimmed == "topo:coordination" {
				return true, nil
			}
		}
	}
	return false, nil
}

func addAllowEmptyStartableLeafFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "allow-empty-startable-leaf", false, "allow updates that temporarily leave zero workable startable leaf tickets (emergency override)")
}
