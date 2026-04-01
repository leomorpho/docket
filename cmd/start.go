package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/hooks"
	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
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
In --auto mode, it runs the managed ticket flow and continues to the next ticket after each completion.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		deps := newRuntimeDeps(repo)
		s := deps.store
		ctx := context.Background()
		if err := s.SyncIndex(ctx); err != nil {
			return fmt.Errorf("syncing index: %w", err)
		}

		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		if startRun || startAuto {
			return runStartManaged(cmd, ctx, s, cfg)
		}
		ns := runstate.New(runtimeNamespaceRoot(repo))
		activeWorkflowHash, _, err := ns.GetActiveWorkflowHash(repo)
		if err != nil {
			return fmt.Errorf("checking active workflow policy: %w", err)
		}

		// 1. Select the next ticket
		t, err := selectNextTicket(ctx, s, cfg)
		if err != nil {
			return err
		}
		resumeMode := ""
		var autoHealResult *queueHealResult
		if t == nil && shouldAutoHealQueueOnStart() {
			heal, healErr := executeQueueHeal(ctx, s, cfg, true)
			if healErr != nil {
				return healErr
			}
			autoHealResult = &heal
			if heal.Applied {
				t, err = selectNextTicket(ctx, s, cfg)
				if err != nil {
					return err
				}
			}
		}
		if t == nil {
			resume, resumeErr := selectResumableActiveTicket(ctx, s, cfg)
			if resumeErr != nil {
				return resumeErr
			}
			if resume != nil {
				t = resume
				resumeMode = "active"
			}
		}
		if t == nil {
			diagnosis, diagErr := workablepkg.DiagnoseEmpty(ctx, s, cfg)
			if diagErr != nil {
				return diagErr
			}
			message := diagnosis.Summary()
			if autoHealResult != nil {
				message = autoHealResult.Summary + " " + message
			}
			capabilityDigest := buildStartCapabilityDigest(repo)
			quickPath := buildLLMQuickPath()
			agentQuickstart := buildStartAgentQuickstart(repo, "", "")
			if format == "json" {
				printJSON(cmd, map[string]interface{}{
					"ticket":               nil,
					"no_workable_ticket":   true,
					"message":              message,
					"active_workflow_hash": activeWorkflowHash,
					"capability_digest":    capabilityDigest,
					"llm_quick_path":       quickPath,
					"agent_quickstart":     agentQuickstart,
					"queue_heal":           autoHealResult,
					"resume_mode":          resumeMode,
				})
				return nil
			}
			if autoHealResult != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Auto-heal: %s\n", autoHealResult.Summary)
			}
			renderStartNoTicketIntro(cmd, message, capabilityDigest, quickPath, agentQuickstart)
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
		worktreePath := repo
		if resumeMode == "active" {
			if format != "json" {
				fmt.Fprintf(cmd.OutOrStdout(), "Resuming active ticket: %s\n", t.ID)
			}
			if run, ok, runErr := ns.GetRunManifest(repo, t.ID); runErr == nil && ok {
				if strings.TrimSpace(run.WorktreePath) != "" {
					worktreePath = run.WorktreePath
				}
			}
		} else {
			t, worktreePath, err = deps.workflow.StartTask(ctx, t.ID, actor, cfg)
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
		}
		if worktreePath == "" {
			worktreePath = repo
		}
		if resumeMode != "active" {
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
				return failStart("runtime.record_run_start", fmt.Errorf("recording run manifest: %w", err))
			}
		}
		tokenEstimate, risk, failureCount := routingInputs(t)
		preferredTier := workflow.SelectCapabilityTier(tokenEstimate, risk, failureCount)
		adapter := workflow.DefaultProviderAdapter()
		model, decision, routeErr := workflow.ResolveModelForTask(adapter, preferredTier)
		if routeErr != nil {
			return failStart("workflow.resolve_model_route", fmt.Errorf("resolving model route: %w", routeErr))
		}
		if err := ns.RecordRunRoutingDecision(repo, t.ID, string(decision.SelectedTier), adapter.ProviderName(), model.ID, decision.Rationale); err != nil {
			if resumeMode == "active" && strings.Contains(strings.ToLower(err.Error()), "run manifest missing") {
				// Resumed active tickets may come from legacy/manual states without a managed run manifest.
				// Keep start usable by continuing without routing metadata persistence.
			} else {
				return failStart("runtime.record_run_routing", fmt.Errorf("recording run routing metadata: %w", err))
			}
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
			payload := map[string]interface{}{
				"ticket":               t,
				"model_tier":           decision.SelectedTier,
				"model_id":             model.ID,
				"routing_rationale":    decision.Rationale,
				"active_workflow_hash": activeWorkflowHash,
				"managed_run_branch":   managedBranch,
				"managed_run_worktree": worktreePath,
				"agent_instruction":    instruction,
				"capability_digest":    capabilityDigest,
				"learn_replay":         learnReplay,
				"llm_quick_path":       quickPath,
				"agent_quickstart":     agentQuickstart,
				"resume_mode":          resumeMode,
			}
			if autoHealResult != nil {
				payload["queue_heal"] = autoHealResult
			}
			printJSON(cmd, payload)
			return nil
		}
		if autoHealResult != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Auto-heal: %s\n", autoHealResult.Summary)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\n=== Agent Prompt ===\n")
		fmt.Fprintf(cmd.OutOrStdout(), "You have started working on ticket: %s\n", t.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", t.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", t.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "Model tier: %s (%s)\n", decision.SelectedTier, model.ID)
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
	if err := s.SyncIndex(ctx); err != nil {
		return fmt.Errorf("syncing index: %w", err)
	}
	svc := newRunOrchestrator(repo, runReviewEnabled())
	if startAuto {
		summary, err := executeCycleRun(cmd, func(ctx context.Context) (agentrun.CycleSummary, error) {
			return svc.RunNext(ctx)
		})
		if err != nil {
			return err
		}
		return renderCycleSummary(cmd, summary)
	}

	t, err := selectNextTicket(ctx, s, cfg)
	if err != nil {
		return err
	}
	var autoHealResult *queueHealResult
	if t == nil && shouldAutoHealQueueOnStart() {
		heal, healErr := executeQueueHeal(ctx, s, cfg, true)
		if healErr != nil {
			return healErr
		}
		autoHealResult = &heal
		if heal.Applied {
			t, err = selectNextTicket(ctx, s, cfg)
			if err != nil {
				return err
			}
		}
	}
	if t == nil {
		resume, resumeErr := selectResumableActiveTicket(ctx, s, cfg)
		if resumeErr != nil {
			return resumeErr
		}
		if resume != nil {
			t = resume
		}
	}
	if t == nil {
		diagnosis, diagErr := workablepkg.DiagnoseEmpty(ctx, s, cfg)
		if diagErr != nil {
			return diagErr
		}
		message := diagnosis.Summary()
		if autoHealResult != nil {
			message = autoHealResult.Summary + " " + message
		}
		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket":             nil,
				"no_workable_ticket": true,
				"message":            message,
				"queue_heal":         autoHealResult,
			})
			return nil
		}
		if autoHealResult != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Auto-heal: %s\n", autoHealResult.Summary)
		}
		fmt.Fprintln(cmd.OutOrStdout(), message)
		return nil
	}

	summary, err := executeTicketRun(cmd, t.ID, func(ctx context.Context) (agentrun.TicketRunSummary, error) {
		return svc.RunTicket(ctx, t.ID)
	})
	if err != nil {
		return err
	}
	return renderTicketRunSummary(cmd, summary)
}

func renderStartNoTicketIntro(cmd *cobra.Command, message string, capabilityDigest startCapabilityDigest, quickPath llmQuickPath, agentQuickstart startAgentQuickstart) {
	fmt.Fprintln(cmd.OutOrStdout(), message)
	fmt.Fprintf(cmd.OutOrStdout(), "\n=== Docket Intro ===\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Docket is ready even without an active ticket.\n")
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
	claimedAtSelection, err := listClaimedTicketIDs(s.RepoRoot)
	if err != nil {
		return nil, err
	}
	tickets, err := workableTickets(ctx, s, cfg, store.Filter{})
	if err != nil {
		return nil, err
	}

	var claimedFallback *ticket.Ticket
	for _, t := range tickets {
		if claimedAtSelection[t.ID] {
			if claimedFallback == nil {
				claimedFallback = t
			}
			continue
		}
		cl, claimErr := lookupClaimIfAvailable(s.RepoRoot, t.ID)
		if claimErr != nil {
			return nil, claimErr
		}
		if cl == nil {
			return t, nil
		}
		if claimedFallback == nil {
			claimedFallback = t
		}
	}

	return claimedFallback, nil
}

func selectResumableActiveTicket(ctx context.Context, s *local.Store, cfg *ticket.Config) (*ticket.Ticket, error) {
	if diagnosis, diagErr := workablepkg.DiagnoseEmpty(ctx, s, cfg); diagErr == nil {
		for _, blocker := range diagnosis.TopBlockers {
			full, getErr := s.GetTicket(ctx, blocker.ID)
			if getErr != nil {
				return nil, getErr
			}
			if full == nil || full.StartedAt.IsZero() {
				continue
			}
			if cfg.StateHasRole(string(full.State), "active") {
				return full, nil
			}
		}
	}

	activeStates := cfg.StatesWithRole("active")
	if len(activeStates) == 0 {
		return nil, nil
	}
	filter := store.Filter{States: make([]ticket.State, 0, len(activeStates))}
	for _, state := range activeStates {
		filter.States = append(filter.States, ticket.State(state))
	}
	tickets, err := s.ListTickets(ctx, filter)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	if len(tickets) == 0 {
		return nil, nil
	}
	started := make([]*ticket.Ticket, 0, len(tickets))
	for _, item := range tickets {
		full, getErr := s.GetTicket(ctx, item.ID)
		if getErr != nil {
			return nil, getErr
		}
		if full == nil || full.StartedAt.IsZero() {
			continue
		}
		started = append(started, full)
	}
	if len(started) == 0 {
		return nil, nil
	}
	best := started[0]
	bestTime := best.StartedAt
	for _, t := range started[1:] {
		if t.Priority != best.Priority {
			if t.Priority < best.Priority {
				best = t
				bestTime = t.StartedAt
			}
			continue
		}
		candidateTime := t.StartedAt
		if candidateTime.Before(bestTime) || (candidateTime.Equal(bestTime) && t.ID < best.ID) {
			best = t
			bestTime = candidateTime
		}
	}
	return best, nil
}

func listClaimedTicketIDs(repoRoot string) (map[string]bool, error) {
	ids := make(map[string]bool)
	dirs := make([]string, 0, 2)
	if dir, err := claim.GetClaimsDir(repoRoot); err == nil {
		dirs = append(dirs, dir)
	} else if !strings.Contains(err.Error(), "not a git repository") {
		return nil, err
	}
	dirs = append(dirs, filepath.Join(repoRoot, ".git", "docket", "claims"))

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			ids[strings.TrimSuffix(name, ".json")] = true
		}
	}
	return ids, nil
}

func lookupClaimIfAvailable(repoRoot, ticketID string) (*claim.ClaimMetadata, error) {
	cl, err := claim.GetClaim(repoRoot, ticketID)
	if err == nil && cl != nil {
		return cl, nil
	}
	if err != nil && !strings.Contains(err.Error(), "not a git repository") {
		return nil, err
	}

	// Fallback: detect raw claim files even when claim metadata lookup cannot
	// resolve the repository path in the current execution context.
	dir, dirErr := claim.GetClaimsDir(repoRoot)
	if dirErr != nil {
		if strings.Contains(dirErr.Error(), "not a git repository") {
			return nil, nil
		}
		return nil, dirErr
	}
	path := filepath.Join(dir, ticketID+".json")
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			legacyPath := filepath.Join(repoRoot, ".git", "docket", "claims", ticketID+".json")
			legacyData, legacyErr := os.ReadFile(legacyPath)
			if legacyErr != nil {
				if os.IsNotExist(legacyErr) {
					return nil, nil
				}
				return nil, legacyErr
			}
			data = legacyData
		} else {
			return nil, readErr
		}
	}
	var fallback claim.ClaimMetadata
	if err := json.Unmarshal(data, &fallback); err == nil {
		return &fallback, nil
	}
	return &claim.ClaimMetadata{}, nil
}

func init() {
	startCmd.Flags().BoolVar(&startAuto, "auto", false, "automatically continue to the next ticket after completion; implies --run")
	startCmd.Flags().BoolVar(&startRun, "run", false, "run the next workable ticket through the Codex flow instead of printing a prompt")
	startCmd.Flags().BoolVar(&runWatch, "watch", false, "open the interactive managed-run dashboard while this run is active")
	startCmd.Flags().BoolVar(&runEnableReview, "review", false, "run the optional reviewer pass and capped fix loop after implementer validation")
	startCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "compatibility alias; reviewer pass is disabled by default")
	_ = startCmd.Flags().MarkHidden("review")
	_ = startCmd.Flags().MarkHidden("no-review")
	startCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode (exec or session)")
	startCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "run a managed-run health check after this much time without new Codex output")
	rootCmd.AddCommand(startCmd)
}

func shouldAutoHealQueueOnStart() bool {
	if !isAutomationMode() {
		return false
	}
	raw := strings.TrimSpace(os.Getenv("DOCKET_START_AUTO_HEAL"))
	if raw == "" {
		return true
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
