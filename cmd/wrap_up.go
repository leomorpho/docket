package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

type wrapUpCheck struct {
	ID      string   `json:"id"`
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type wrapUpReport struct {
	TicketID  string        `json:"ticket_id"`
	Ready     bool          `json:"ready"`
	State     ticket.State  `json:"state"`
	Checks    []wrapUpCheck `json:"checks"`
	NextSteps []string      `json:"next_steps,omitempty"`
}

var wrapUpCmd = &cobra.Command{
	Use:   "wrap-up <TKT-NNN>",
	Short: "Run closeout readiness checks for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		if normalized, ok := ticket.NormalizeID(id); ok {
			id = normalized
		}

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			cfg = ticket.DefaultConfig()
		}

		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		report, err := buildWrapUpReport(context.Background(), s, cfg, t)
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, report)
			return nil
		}
		renderWrapUpHuman(cmd, report)
		return nil
	},
}

func buildWrapUpReport(ctx context.Context, s *local.Store, cfg *ticket.Config, t *ticket.Ticket) (wrapUpReport, error) {
	checks := []wrapUpCheck{}
	next := []string{}

	acComplete := t.ACComplete()
	checks = append(checks, wrapUpCheck{
		ID:      "ac_complete",
		OK:      acComplete,
		Message: "Acceptance criteria are complete.",
	})
	if !acComplete {
		next = append(next, fmt.Sprintf("Complete remaining AC items: docket ac list %s", t.ID))
	}

	handoffMissing := []string{}
	for _, failure := range closeoutReadinessFailures(t, cfg) {
		if strings.HasPrefix(failure, "handoff") {
			handoffMissing = append(handoffMissing, failure)
		}
	}
	handoffOK := len(handoffMissing) == 0
	checks = append(checks, wrapUpCheck{
		ID:      "handoff_ready",
		OK:      handoffOK,
		Message: "Handoff is present with required closeout sections.",
		Details: handoffMissing,
	})
	if !handoffOK {
		next = append(next, fmt.Sprintf("Add or complete handoff sections: docket update %s --handoff -", t.ID))
	}

	unresolvedBlockers := []string{}
	for _, blockerID := range t.BlockedBy {
		b, err := s.GetTicket(ctx, blockerID)
		if err != nil {
			return wrapUpReport{}, err
		}
		if b == nil || cfg.BlocksDependents(b.State) {
			unresolvedBlockers = append(unresolvedBlockers, blockerID)
		}
	}
	blockersOK := len(unresolvedBlockers) == 0
	checks = append(checks, wrapUpCheck{
		ID:      "blockers_cleared",
		OK:      blockersOK,
		Message: "No unresolved blockers.",
		Details: unresolvedBlockers,
	})
	if !blockersOK {
		next = append(next, fmt.Sprintf("Resolve or remove blockers: docket update %s --unblock <TKT-NNN>", t.ID))
	}

	activeState := activeWorkflowState(cfg)
	closeoutState, closeoutStateOK := nextCloseoutState(cfg, t.State)
	stateReady := cfg.StateHasRole(string(t.State), "active") || cfg.StateHasRole(string(t.State), "completed")
	checks = append(checks, wrapUpCheck{
		ID:      "state_ready",
		OK:      stateReady,
		Message: "Ticket is in an active or completed workflow state.",
		Details: []string{string(t.State)},
	})
	if !stateReady {
		next = append(next, fmt.Sprintf("Move ticket into active work before wrap-up: docket update %s --state %s", t.ID, activeState))
	}

	ready := true
	for _, check := range checks {
		if !check.OK {
			ready = false
			break
		}
	}
	if ready && !cfg.StateHasRole(string(t.State), "completed") && closeoutStateOK {
		next = append(next, fmt.Sprintf("Advance toward validated/completed state: docket update %s --state %s", t.ID, closeoutState))
	}

	return wrapUpReport{
		TicketID:  t.ID,
		Ready:     ready,
		State:     t.State,
		Checks:    checks,
		NextSteps: next,
	}, nil
}

func renderWrapUpHuman(cmd *cobra.Command, report wrapUpReport) {
	status := "NOT READY"
	if report.Ready {
		status = "READY"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Wrap-up for %s: %s\n", report.TicketID, status)
	fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", report.State)
	for _, check := range report.Checks {
		marker := "[ ]"
		if check.OK {
			marker = "[x]"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", marker, check.Message)
		for _, detail := range check.Details {
			if strings.TrimSpace(detail) == "" {
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", detail)
		}
	}
	if len(report.NextSteps) == 0 {
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	for i, step := range report.NextSteps {
		fmt.Fprintf(cmd.OutOrStdout(), "%d. %s\n", i+1, step)
	}
}

func init() {
	rootCmd.AddCommand(wrapUpCmd)
}

func nextCloseoutState(cfg *ticket.Config, current ticket.State) (string, bool) {
	if cfg == nil {
		return completedWorkflowState(cfg), true
	}
	currentState := strings.TrimSpace(string(current))
	if currentState == "" {
		return completedWorkflowState(cfg), true
	}
	if cfg.StateHasRole(currentState, "completed") {
		return currentState, true
	}

	type queueItem struct {
		State string
		First string
	}
	visited := map[string]bool{currentState: true}
	queue := []queueItem{{State: currentState}}
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		stateCfg, ok := cfg.States[item.State]
		if !ok {
			continue
		}
		for _, nextState := range stateCfg.Next {
			if visited[nextState] {
				continue
			}
			visited[nextState] = true
			firstHop := item.First
			if firstHop == "" {
				firstHop = nextState
			}
			if cfg.StateHasRole(nextState, "completed") {
				return firstHop, true
			}
			queue = append(queue, queueItem{State: nextState, First: firstHop})
		}
	}
	return completedWorkflowState(cfg), false
}
