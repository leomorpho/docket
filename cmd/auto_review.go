package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func maybeAutoTransitionReviewReady(
	ctx context.Context,
	out io.Writer,
	s *local.Store,
	cfg *ticket.Config,
	t *ticket.Ticket,
	actor string,
) (*ticket.Ticket, bool) {
	if t == nil || cfg == nil || !cfg.StateHasRole(string(t.State), "active") {
		return t, false
	}
	reviewState := ticket.State(nextStateForRole(cfg, t.State, "review", "in-review"))

	failures := reviewReadinessFailures(t, cfg)
	if len(failures) > 0 {
		fmt.Fprintf(out, "Auto-review skipped for %s: %s\n", t.ID, strings.Join(failures, "; "))
		return t, false
	}

	if err := ticket.ValidateTransition(cfg, t.State, reviewState); err != nil {
		fmt.Fprintf(out, "Auto-review skipped for %s: %v\n", t.ID, err)
		return t, false
	}

	if err := enforceManagedRunCommitLinkage(t.ID, reviewState); err != nil {
		fmt.Fprintf(out, "Auto-review skipped for %s: %v\n", t.ID, err)
		return t, false
	}

	deps := newRuntimeDeps(repo)
	fromState := t.State
	if _, err := deps.workflow.FinishTask(ctx, t.ID, cfg); err != nil {
		fmt.Fprintf(out, "Auto-review skipped for %s: %v\n", t.ID, err)
		return t, false
	}

	updated, err := s.GetTicket(ctx, t.ID)
	if err != nil || updated == nil {
		fmt.Fprintf(out, "Auto-review transition for %s succeeded but reload failed: %v\n", t.ID, err)
		return t, false
	}

	emitStateTransitionEvent(
		out,
		"update.auto_review",
		updated.ID,
		actor,
		string(fromState),
		string(updated.State),
		"auto review-ready transition",
		[]string{
			"state_transition_validated",
			"ac_complete",
			"handoff_sections_present",
			"managed_run_commit_linkage",
		},
	)
	_ = releaseLockForTicket(repo, updated.ID)
	fmt.Fprintf(out, "Auto-transitioned %s: state %s → %s (review-ready)\n", updated.ID, fromState, updated.State)
	return updated, true
}

func reviewReadinessFailures(t *ticket.Ticket, cfg *ticket.Config) []string {
	failures := []string{}
	if !t.ACComplete() {
		failures = append(failures, "acceptance criteria incomplete")
	}
	handoff := strings.TrimSpace(t.Handoff)
	if handoff == "" {
		failures = append(failures, "handoff missing")
		return failures
	}
	lower := strings.ToLower(handoff)
	for _, section := range cfg.HandoffSections {
		if strings.Contains(lower, strings.ToLower(section)) {
			continue
		}
		failures = append(failures, fmt.Sprintf("handoff missing section %s", section))
	}
	return failures
}
