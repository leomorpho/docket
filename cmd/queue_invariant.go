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
	if allow || s == nil || cfg == nil {
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
	if allow || beforeCount <= 0 || s == nil || cfg == nil {
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

func addAllowEmptyStartableLeafFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "allow-empty-startable-leaf", false, "allow updates that temporarily leave zero workable startable leaf tickets (emergency override)")
}
