package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/leomorpho/docket/internal/hooks"
	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	startAuto bool
	startRun  bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Automatically pick up and start the next best ticket",
	Long: `Identify the next unblocked high-priority ticket in an open state,
claims it, creates a worktree for it if needed, and transitions it to the repo's configured active work state.
In --auto mode, it will continue to the next ticket after each completion.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := newRuntimeDeps(repo)
		s := deps.store
		ctx := context.Background()

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		if startRun {
			return runStartManaged(cmd, ctx, s, cfg)
		}
		ns := security.NewRepoNamespaceStore(docketHome)
		activeWorkflowHash, active, err := ns.GetActiveWorkflowHash(repo)
		if err != nil {
			return fmt.Errorf("checking active workflow policy: %w", err)
		}
		runtimePolicyMode := "unsecured"
		runtimePolicyMessage := "No active workflow.lock is approved; privileged and terminal transitions remain blocked."
		if active {
			runtimePolicyMode = "approved-lock"
			runtimePolicyMessage = fmt.Sprintf("Using approved workflow.lock %s.", activeWorkflowHash)
		}

		// 1. Select the next ticket
		t, err := selectNextTicket(ctx, s, cfg)
		if err != nil {
			return err
		}
		if t == nil {
			capabilityDigest := buildStartCapabilityDigest(repo)
			quickPath := buildLLMQuickPath()
			agentQuickstart := buildStartAgentQuickstart(repo, "", "")
			if format == "json" {
				printJSON(cmd, map[string]interface{}{
					"ticket":               nil,
					"no_workable_ticket":   true,
					"message":              fmt.Sprintf("No workable tickets found. Startable states in current config: %s.", startableStatesSummary(cfg)),
					"runtime_policy_mode":  runtimePolicyMode,
					"runtime_policy_note":  runtimePolicyMessage,
					"active_workflow_hash": activeWorkflowHash,
					"capability_digest":    capabilityDigest,
					"llm_quick_path":       quickPath,
					"agent_quickstart":     agentQuickstart,
				})
				return nil
			}
			renderStartNoTicketIntro(cmd, cfg, runtimePolicyMode, runtimePolicyMessage, capabilityDigest, quickPath, agentQuickstart)
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
		recorder := lifecycleStart(cmd.ErrOrStderr(), "start", t.ID, actor)
		runStatus := lifecycle.StatusOK
		defer func() {
			lifecycleRunEnd(cmd.ErrOrStderr(), recorder, runStatus)
		}()

		failStart := func(tool string, err error) error {
			runStatus = lifecycle.StatusFailed
			lifecycleToolFailure(cmd.ErrOrStderr(), recorder, lifecyclePhaseStartWorkflow, tool, err)
			lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseStartWorkflow, lifecycle.StatusFailed)
			return err
		}

		previousState := t.State
		t, worktreePath, err := deps.workflow.StartTask(ctx, t.ID, actor, cfg)
		if err != nil {
			return failStart("workflow.start_task", err)
		}
		emitStateTransitionEvent(
			cmd.ErrOrStderr(),
			"start",
			t.ID,
			actor,
			string(previousState),
			string(t.State),
			"start selected next ticket",
			[]string{"state_transition_validated", "next_ticket_selected"},
		)
		if worktreePath == "" {
			worktreePath = repo
		}
		hookManager := hooks.NewManager()
		hooks.RegisterCoreHooks(hookManager)
		targetState := activeWorkflowState(cfg)
		advisory, hookErr := hookManager.Run(hooks.EventRunStart, hooks.Context{
			Repo:         repo,
			TicketID:     t.ID,
			Actor:        actor,
			ManagedRun:   strings.HasPrefix(actor, "agent:"),
			WorktreePath: worktreePath,
			Branch:       "docket/" + t.ID,
			TargetState:  targetState,
		})
		for _, msg := range advisory {
			fmt.Fprintf(cmd.OutOrStdout(), "hook advisory: %s\n", msg)
		}
		if hookErr != nil {
			return failStart("hooks.run_start", fmt.Errorf("start hook failed: %w", hookErr))
		}
		if err := ns.RecordRunStart(repo, t.ID, actor, worktreePath, "docket/"+t.ID, activeWorkflowHash); err != nil {
			return failStart("security.record_run_start", fmt.Errorf("recording run manifest: %w", err))
		}
		tokenEstimate, risk, failureCount := routingInputs(t)
		preferredTier := workflow.SelectCapabilityTier(tokenEstimate, risk, failureCount)
		adapter := workflow.DefaultProviderAdapter()
		model, decision, routeErr := workflow.ResolveModelForTask(adapter, preferredTier)
		if routeErr != nil {
			return failStart("workflow.resolve_model_route", fmt.Errorf("resolving model route: %w", routeErr))
		}
		if err := ns.RecordRunRoutingDecision(repo, t.ID, string(decision.SelectedTier), adapter.ProviderName(), model.ID, decision.Rationale); err != nil {
			return failStart("security.record_run_routing", fmt.Errorf("recording run routing metadata: %w", err))
		}
		lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseStartWorkflow, lifecycle.StatusOK)
		instruction := startInstruction(t.ID)
		capabilityDigest := buildStartCapabilityDigest(repo)
		learnReplay := buildLearnReplay(repo, t, 3)
		quickPath := buildLLMQuickPath()
		managedBranch := "docket/" + t.ID
		agentQuickstart := buildStartAgentQuickstart(repo, managedBranch, worktreePath)

		// 3. Provide the Agent Prompt
		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket":               t,
				"model_tier":           decision.SelectedTier,
				"model_id":             model.ID,
				"routing_rationale":    decision.Rationale,
				"runtime_policy_mode":  runtimePolicyMode,
				"runtime_policy_note":  runtimePolicyMessage,
				"active_workflow_hash": activeWorkflowHash,
				"managed_run_branch":   managedBranch,
				"managed_run_worktree": worktreePath,
				"agent_instruction":    instruction,
				"capability_digest":    capabilityDigest,
				"learn_replay":         learnReplay,
				"llm_quick_path":       quickPath,
				"agent_quickstart":     agentQuickstart,
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\n=== Agent Prompt ===\n")
		fmt.Fprintf(cmd.OutOrStdout(), "You have started working on ticket: %s\n", t.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", t.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", t.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "Model tier: %s (%s)\n", decision.SelectedTier, model.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Runtime policy: %s\n", runtimePolicyMode)
		fmt.Fprintf(cmd.OutOrStdout(), "Policy note: %s\n", runtimePolicyMessage)
		fmt.Fprintf(cmd.OutOrStdout(), "Managed run binding: branch=%s | worktree=%s\n", managedBranch, worktreePath)
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", renderStartAgentQuickstartHuman(agentQuickstart))
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", renderStartCapabilityDigestHuman(capabilityDigest))
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"LLM quick path:\n- %s\n- %s\n- %s\n- %s\n",
			quickPath.TicketApply,
			quickPath.BacklogApply,
			quickPath.ProofAttach,
			quickPath.ProofVerify,
		)
		fmt.Fprintf(cmd.OutOrStdout(), "Automation: %s\n", quickPath.AutomationHint)
		if len(learnReplay) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Learn replay: none\n")
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Learn replay (top %d):\n", len(learnReplay))
			for _, rule := range learnReplay {
				fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s\n", rule.Category, rule.Rule)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nAcceptance Criteria:\n")
		for _, ac := range t.AC {
			status := "[ ]"
			if ac.Done {
				status = "[x]"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s %s\n", status, ac.Description)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nInstruction:\n%s\n", instruction)
		fmt.Fprintf(cmd.OutOrStdout(), "====================\n")

		return nil
	},
}

func runStartManaged(cmd *cobra.Command, ctx context.Context, s *local.Store, cfg *ticket.Config) error {
	svc := newRunOrchestrator(repo, runReviewEnabled())
	if startAuto {
		summary, err := svc.RunNext(ctx)
		if err != nil {
			return err
		}
		return renderCycleSummary(cmd, summary)
	}

	t, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		return err
	}
	if t == nil {
		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket":             nil,
				"no_workable_ticket": true,
				"message":            fmt.Sprintf("No workable tickets found. Startable states in current config: %s.", startableStatesSummary(cfg)),
			})
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "No workable tickets found. Startable states in current config: %s.\n", startableStatesSummary(cfg))
		return nil
	}

	summary, err := svc.RunTicket(ctx, t.ID)
	if err != nil {
		return err
	}
	return renderTicketRunSummary(cmd, summary)
}

func renderStartNoTicketIntro(cmd *cobra.Command, cfg *ticket.Config, runtimePolicyMode, runtimePolicyMessage string, capabilityDigest startCapabilityDigest, quickPath llmQuickPath, agentQuickstart startAgentQuickstart) {
	fmt.Fprintf(cmd.OutOrStdout(), "No workable tickets found. Startable states in current config: %s.\n", startableStatesSummary(cfg))
	fmt.Fprintf(cmd.OutOrStdout(), "\n=== Docket Intro ===\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Docket is ready even without an active ticket.\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Runtime policy: %s\n", runtimePolicyMode)
	fmt.Fprintf(cmd.OutOrStdout(), "Policy note: %s\n", runtimePolicyMessage)
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", renderStartAgentQuickstartHuman(agentQuickstart))
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", renderStartCapabilityDigestHuman(capabilityDigest))
	fmt.Fprintf(
		cmd.OutOrStdout(),
		"LLM quick path:\n- %s\n- %s\n- %s\n- %s\n",
		quickPath.TicketApply,
		quickPath.BacklogApply,
		quickPath.ProofAttach,
		quickPath.ProofVerify,
	)
	fmt.Fprintf(cmd.OutOrStdout(), "Automation: %s\n", quickPath.AutomationHint)
	fmt.Fprintf(cmd.OutOrStdout(), "====================\n")
}

func startInstruction(ticketID string) string {
	lines := []string{
		fmt.Sprintf("Work only ticket %s in this run.", ticketID),
		"Use test-driven development.",
		"Analyze requirements, write or update tests first, then implement the smallest passing change.",
		"Before moving on, update ticket state/comments with `docket` commands and commit with a `Ticket: <TKT-NNN>` trailer.",
		fmt.Sprintf("Use `Ticket: %s` in your commit trailer for this ticket.", ticketID),
	}
	return strings.Join(lines, "\n")
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
	tickets, err := workableTickets(ctx, s, cfg, store.Filter{})
	if err != nil {
		return nil, err
	}

	for _, t := range tickets {
		return t, nil
	}

	return nil, nil
}

func init() {
	startCmd.Flags().BoolVar(&startAuto, "auto", false, "automatically continue to the next ticket after completion")
	startCmd.Flags().BoolVar(&startRun, "run", false, "run the next workable ticket through the Codex flow instead of printing a prompt")
	startCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "skip the default reviewer pass and capped fix-review loop")
	startCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "mark the managed run hung after this much time without new Codex output")
	rootCmd.AddCommand(startCmd)
}
