package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/leomorpho/docket/internal/hooks"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	startAuto bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Automatically pick up and start the next best ticket",
	Long: `Identify the next unblocked high-priority ticket in an open state,
claims it, creates a worktree for it if needed, and transitions it to 'in-progress'.
In --auto mode, it will continue to the next ticket after each completion.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := newRuntimeDeps(repo)
		s := deps.store
		ctx := context.Background()

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		ns := security.NewRepoNamespaceStore(docketHome)
		activeWorkflowHash, active, err := ns.GetActiveWorkflowHash(repo)
		if err != nil {
			return fmt.Errorf("checking active workflow policy: %w", err)
		}
		if !active {
			return fmt.Errorf("no active workflow.lock is approved for this repo. Run `docket workflow lock activate --ticket TKT-NNN` in secure mode")
		}

		// 1. Select the next ticket
		t, err := selectNextTicket(ctx, s, cfg)
		if err != nil {
			return err
		}
		if t == nil {
			fmt.Fprintln(cmd.OutOrStdout(), "No unblocked tickets found in open states (backlog, todo).")
			return nil
		}

		// Load full ticket details (ListTickets only returns metadata)
		t, err = s.GetTicket(ctx, t.ID)
		if err != nil {
			return fmt.Errorf("loading ticket details: %w", err)
		}

		actor := detectActor()
		if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
			actor = "agent:" + agentID
		}

		t, worktreePath, err := deps.workflow.StartTask(ctx, t.ID, actor, cfg)
		if err != nil {
			return err
		}
		if worktreePath == "" {
			worktreePath = repo
		}
		hookManager := hooks.NewManager()
		hooks.RegisterCoreHooks(hookManager)
		advisory, hookErr := hookManager.Run(hooks.EventRunStart, hooks.Context{
			Repo:         repo,
			TicketID:     t.ID,
			Actor:        actor,
			ManagedRun:   strings.HasPrefix(actor, "agent:"),
			WorktreePath: worktreePath,
			Branch:       "docket/" + t.ID,
			TargetState:  "in-progress",
		})
		for _, msg := range advisory {
			fmt.Fprintf(cmd.OutOrStdout(), "hook advisory: %s\n", msg)
		}
		if hookErr != nil {
			return fmt.Errorf("start hook failed: %w", hookErr)
		}
		if err := ns.RecordRunStart(repo, t.ID, actor, worktreePath, "docket/"+t.ID, activeWorkflowHash); err != nil {
			return fmt.Errorf("recording run manifest: %w", err)
		}
		tokenEstimate, risk, failureCount := routingInputs(t)
		preferredTier := workflow.SelectCapabilityTier(tokenEstimate, risk, failureCount)
		adapter := workflow.DefaultProviderAdapter()
		model, decision, routeErr := workflow.ResolveModelForTask(adapter, preferredTier)
		if routeErr != nil {
			return fmt.Errorf("resolving model route: %w", routeErr)
		}
		if err := ns.RecordRunRoutingDecision(repo, t.ID, string(decision.SelectedTier), adapter.ProviderName(), model.ID, decision.Rationale); err != nil {
			return fmt.Errorf("recording run routing metadata: %w", err)
		}

		// 3. Provide the Agent Prompt
		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket":            t,
				"model_tier":        decision.SelectedTier,
				"model_id":          model.ID,
				"routing_rationale": decision.Rationale,
				"agent_instruction": "Analyze the requirements and implement the changes. Use 'docket' tools to track your progress.",
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\n=== Agent Prompt ===\n")
		fmt.Fprintf(cmd.OutOrStdout(), "You have started working on ticket: %s\n", t.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", t.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", t.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "Model tier: %s (%s)\n", decision.SelectedTier, model.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "\nAcceptance Criteria:\n")
		for _, ac := range t.AC {
			status := "[ ]"
			if ac.Done {
				status = "[x]"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s %s\n", status, ac.Description)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nInstruction: Analyze the requirements and implement the changes. Use 'docket' tools to track your progress.\n")
		fmt.Fprintf(cmd.OutOrStdout(), "====================\n")

		return nil
	},
}

func routingInputs(t *ticket.Ticket) (tokenEstimate int, risk string, failureCount int) {
	risk = "low"
	for _, l := range t.Labels {
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "security", "user-facing", "human-only":
			risk = "high"
		}
	}
	var sb strings.Builder
	sb.WriteString(t.Title)
	sb.WriteString(" ")
	sb.WriteString(t.Description)
	for _, ac := range t.AC {
		sb.WriteString(" ")
		sb.WriteString(ac.Description)
	}
	tokenEstimate = len(strings.Fields(sb.String())) * 2
	for _, c := range t.Comments {
		body := strings.ToLower(c.Body)
		if strings.Contains(body, "fail") || strings.Contains(body, "retry") {
			failureCount++
		}
	}
	return tokenEstimate, risk, failureCount
}

func selectNextTicket(ctx context.Context, s *local.Store, cfg *ticket.Config) (*ticket.Ticket, error) {
	// Candidates are open states that are NOT in-progress
	var candidates []ticket.State
	for _, stateName := range cfg.OpenStates() {
		st := ticket.State(stateName)
		if st != "in-progress" {
			candidates = append(candidates, st)
		}
	}

	f := store.Filter{
		States:        candidates,
		OnlyUnblocked: true,
	}

	tickets, err := s.ListTickets(ctx, f)
	if err != nil {
		return nil, err
	}

	for _, t := range tickets {
		if err := ticket.ValidateTransition(cfg, t.State, ticket.State("in-progress")); err == nil {
			return t, nil
		}
	}

	return nil, nil
}

func init() {
	startCmd.Flags().BoolVar(&startAuto, "auto", false, "automatically continue to the next ticket after completion")
	rootCmd.AddCommand(startCmd)
}
