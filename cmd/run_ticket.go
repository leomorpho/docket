package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/agentrun/codex"
	"github.com/leomorpho/docket/internal/agentrun/monitor"
	"github.com/leomorpho/docket/internal/agentrun/orchestrate"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/agentrun/selector"
	runvalidate "github.com/leomorpho/docket/internal/agentrun/validate"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/tui"
	"github.com/leomorpho/docket/internal/vcs"
	workablepkg "github.com/leomorpho/docket/internal/workable"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/leomorpho/docket/internal/workspace"
	"github.com/spf13/cobra"
)

const DefaultRunInactivityTimeout = 2 * time.Minute

var (
	runEnableReview    bool
	runDisableReview   bool
	runInactivityLimit time.Duration
	runManagedAdapter  string
	runWatch           bool
	runWatchMouse      bool
	runWorkspace       bool
	runWatchRetryDelay = 2 * time.Second
)

var newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
	return newRunOrchestratorWithMode(repoRoot, enableReview, managedRunAdapterMode())
}

var managedRunTitleWriter = writeTerminalTitle

var newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
	store := local.New(repoRoot)
	wf := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), newRuntimeDeps(repoRoot).claimer)
	runtimeStore := runruntime.New(repoRoot)
	adapter := managedRunAdapter(mode)
	validator := runvalidate.New(runvalidate.Dependencies{
		RepoRoot: repoRoot,
		Store:    store,
		Workflow: wf,
		Runtime:  runtimeStore,
	})
	deps := orchestrate.Dependencies{
		RepoRoot:  repoRoot,
		Actor:     runActor(),
		Store:     store,
		Workflow:  wf,
		Namespace: runstate.New(runtimeNamespaceRoot(repoRoot)),
		Adapter:   adapter,
		Monitor:   monitor.New(monitor.Dependencies{Runtime: runtimeStore}),
		Validator: validator,
		Selector:  selector.New(selector.Dependencies{Store: store, LoadConfig: ticket.LoadConfig}),
		Runtime:   runtimeStore,
		Timeout:   runInactivityLimitOrDefault(),
	}
	if enableReview {
		deps.Reviewer = adapter
	}
	return orchestrate.New(deps)
}

func managedRunAdapterMode() string {
	mode := strings.TrimSpace(runManagedAdapter)
	if mode == "" {
		return "session"
	}
	return mode
}

func managedRunAdapter(mode string) agentrun.Adapter {
	switch strings.TrimSpace(mode) {
	case "session":
		return codex.NewSessionRunner()
	default:
		return codex.NewRunner()
	}
}

type liveRunLogStreamer struct {
	cmd       *cobra.Command
	store     *runruntime.Store
	mu        sync.Mutex
	seen      map[string]int
	announced map[string]bool
}

func startLiveRunLogs(cmd *cobra.Command, repoRoot string) func() {
	if format == "json" {
		return func() {}
	}
	streamer := &liveRunLogStreamer{
		cmd:       cmd,
		store:     runruntime.New(repoRoot),
		seen:      map[string]int{},
		announced: map[string]bool{},
	}
	streamer.prime()
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			streamer.flush()
			select {
			case <-stopCh:
				streamer.flush()
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		close(stopCh)
		<-doneCh
	}
}

func (s *liveRunLogStreamer) prime() {
	entries, err := os.ReadDir(s.store.RunsRootDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ticketID := entry.Name()
		transcript, err := s.store.LoadTranscript(ticketID)
		if err == nil {
			s.seen[ticketID] = len(transcript)
		}
		if _, ok, err := s.store.LoadStatus(ticketID); err == nil && ok {
			s.announced[ticketID] = true
		}
	}
}

func (s *liveRunLogStreamer) flush() {
	entries, err := os.ReadDir(s.store.RunsRootDir())
	if err != nil {
		return
	}
	ticketIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ticketIDs = append(ticketIDs, entry.Name())
		}
	}
	sort.Strings(ticketIDs)
	for _, ticketID := range ticketIDs {
		s.flushTicket(ticketID)
	}
}

func (s *liveRunLogStreamer) flushTicket(ticketID string) {
	status, ok, err := s.store.LoadStatus(ticketID)
	if err == nil && ok {
		s.mu.Lock()
		if !s.announced[ticketID] {
			s.announced[ticketID] = true
			fmt.Fprintf(s.cmd.OutOrStdout(), "[%s] session=%s active=%t\n", ticketID, status.SessionID, status.Active)
		}
		if status.Active {
			managedRunTitleWriter(s.cmd, formatManagedRunTitle(filepathBase(repo), ticketID, status.CurrentPhase, status.CurrentStep, status.PlannedSteps, status.Active))
		}
		s.mu.Unlock()
	}
	transcript, err := s.store.LoadTranscript(ticketID)
	if err != nil || len(transcript) == 0 {
		return
	}
	s.mu.Lock()
	start := s.seen[ticketID]
	if start > len(transcript) {
		start = 0
	}
	for _, entry := range transcript[start:] {
		fmt.Fprintf(s.cmd.OutOrStdout(), "[%s] %s\n", ticketID, entry.Text)
	}
	s.seen[ticketID] = len(transcript)
	s.mu.Unlock()
}

func filepathBase(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return parts[len(parts)-1]
}

func writeTerminalTitle(cmd *cobra.Command, title string) {
	if format == "json" || strings.TrimSpace(title) == "" {
		return
	}
	if cmd.OutOrStdout() != os.Stdout {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\x1b]0;%s\x07", title)
}

func formatManagedRunTitle(repoName, ticketID, phase string, step, total int, active bool) string {
	parts := []string{}
	if repoName != "" {
		parts = append(parts, repoName)
	}
	if ticketID != "" {
		parts = append(parts, ticketID)
	}
	if phase != "" {
		parts = append(parts, phase)
	} else if active {
		parts = append(parts, "running")
	}
	if total > 0 && step > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", step, total))
	}
	return strings.Join(parts, " • ")
}

func formatTranscriptTimestampLocal(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	local := parsed.In(time.Local)
	return local.Format("Jan 2, 2006 3:04:05 PM MST")
}

func runActor() string {
	actor := detectActor()
	if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
		actor = "agent:" + agentID
	}
	return actor
}

func runReviewEnabled() bool {
	if runDisableReview {
		return false
	}
	return runEnableReview
}

func runInactivityLimitOrDefault() time.Duration {
	if runInactivityLimit > 0 {
		return runInactivityLimit
	}
	return DefaultRunInactivityTimeout
}

var runTicketCmd = &cobra.Command{
	Use:   "run-ticket <TKT-NNN>",
	Short: "Run one ticket through the Codex implementer flow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newRunOrchestrator(repo, runReviewEnabled())
		summary, err := executeTicketRun(cmd, args[0], func(ctx context.Context) (agentrun.TicketRunSummary, error) {
			return svc.RunTicket(ctx, args[0])
		})
		if err != nil {
			return err
		}
		return renderTicketRunSummary(cmd, summary)
	},
}

var runNextCmd = &cobra.Command{
	Use:   "run-next",
	Short: "Run the next logical tickets serially until exhausted or blocked",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newRunOrchestrator(repo, runReviewEnabled())
		summary, err := executeCycleRun(cmd, func(ctx context.Context) (agentrun.CycleSummary, error) {
			return svc.RunNext(ctx)
		})
		if err != nil {
			return err
		}
		return renderCycleSummary(cmd, summary)
	},
}

var runStatusCmd = &cobra.Command{
	Use:   "run-status <TKT-NNN>",
	Short: "Show live status for an active or hung ticket run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := runruntime.New(repo)
		brief, briefOK, err := store.LoadBrief(args[0])
		if err != nil {
			return err
		}
		durableStatus, durableOK, err := store.LoadRecoverableStatus(args[0])
		if err != nil {
			return err
		}
		status, ok, err := store.LoadStatus(args[0])
		if err != nil {
			return err
		}
		if !ok {
			if format == "json" {
				payload := map[string]any{"ticket_id": args[0], "active": false}
				if durableOK {
					payload["session_id"] = durableStatus.SessionID
					payload["last_result_status"] = durableStatus.LastResultStatus
					payload["recoverable"] = true
					payload["resume_command"] = "docket run-resume " + args[0]
				}
				if briefOK {
					payload["brief"] = brief
				}
				printJSON(cmd, payload)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: no active run\n", args[0])
			if briefOK {
				renderRunBriefHuman(cmd, brief)
			}
			if durableOK {
				renderRecoverableBriefHintHuman(cmd, args[0], durableStatus)
			}
			return nil
		}
		showBrief := briefOK && !status.Active
		if format == "json" {
			payload := map[string]any{
				"ticket_id":               status.TicketID,
				"session_id":              status.SessionID,
				"role":                    status.Role,
				"started_at":              status.StartedAt,
				"pid":                     status.PID,
				"active":                  status.Active,
				"hung":                    status.Hung,
				"last_event_at":           status.LastEventAt,
				"last_visible_at":         status.LastVisibleAt,
				"inactivity_timeout":      status.InactivityTimeout,
				"planned_steps":           status.PlannedSteps,
				"current_step":            status.CurrentStep,
				"current_step_title":      status.CurrentStepTitle,
				"current_phase":           status.CurrentPhase,
				"last_marker":             status.LastMarker,
				"last_visible_text":       status.LastVisibleText,
				"last_result_status":      status.LastResultStatus,
				"session_message_count":   status.SessionMessageCount,
				"health_check_count":      status.HealthCheckCount,
				"last_health_check_at":    status.LastHealthCheckAt,
				"last_health_check":       status.LastHealthCheck,
				"last_intervention":       status.LastIntervention,
				"last_intervention_at":    status.LastInterventionAt,
				"consecutive_no_progress": status.ConsecutiveNoProgress,
				"warning":                 status.Warning,
				"recoverable":             isRecoverableManagedRunStatus(status),
			}
			if showBrief {
				payload["brief"] = brief
			}
			printJSON(cmd, payload)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: active=%t hung=%t", status.TicketID, status.Active, status.Hung)
		if isRecoverableManagedRunStatus(status) {
			fmt.Fprintf(cmd.OutOrStdout(), " recoverable=true")
		}
		if status.CurrentStepTitle != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " step=%d/%d %s", status.CurrentStep, status.PlannedSteps, status.CurrentStepTitle)
		}
		if status.CurrentPhase != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " phase=%s", status.CurrentPhase)
		}
		if status.LastVisibleText != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nLast visible: %s", status.LastVisibleText)
		}
		if status.LastEventAt != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nLast event: %s", status.LastEventAt)
		}
		if status.SessionMessageCount > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nSession messages: %d", status.SessionMessageCount)
		}
		if status.HealthCheckCount > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nHealth checks: %d", status.HealthCheckCount)
		}
		if status.LastIntervention != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nLast intervention: %s", status.LastIntervention)
		}
		if status.Warning != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nWarning: %s", status.Warning)
		}
		if isRecoverableManagedRunStatus(status) {
			fmt.Fprintf(cmd.OutOrStdout(), "\nResume with: docket run-resume %s", status.TicketID)
		}
		fmt.Fprintln(cmd.OutOrStdout())
		if showBrief {
			renderRunBriefHuman(cmd, brief)
		}
		return nil
	},
}

var runResumeCmd = &cobra.Command{
	Use:   "run-resume <TKT-NNN>",
	Short: "Resume a hung ticket run in a fresh Codex session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newRunOrchestrator(repo, runReviewEnabled())
		summary, err := executeTicketRun(cmd, args[0], func(ctx context.Context) (agentrun.TicketRunSummary, error) {
			return svc.ResumeTicket(ctx, args[0])
		})
		if err != nil {
			return err
		}
		return renderTicketRunSummary(cmd, summary)
	},
}

var runPingCmd = &cobra.Command{
	Use:   "run-ping <TKT-NNN>",
	Short: "Send a structured status ping into a persisted managed-run session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newRunOrchestrator(repo, runReviewEnabled())
		summary, err := svc.PingTicket(context.Background(), args[0])
		if err != nil {
			return err
		}
		return renderPingSummary(cmd, summary)
	},
}

var runWatchCmd = &cobra.Command{
	Use:   "run-watch [TKT-NNN]",
	Short: "Open the managed-run dashboard and launcher",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := ""
		if len(args) == 1 {
			ticketID = args[0]
		}
		if runWorkspace {
			if ticketID != "" {
				return fmt.Errorf("workspace watch does not support a ticket id argument yet")
			}
			return runWorkspaceWatchDashboard(repo)
		}
		if ticketID != "" {
			return runWatchDashboard(repo, ticketID, nil, false, nil)
		}
		return runWatchDashboard(repo, "", nil, false, runWatchLaunchOptions(repo))
	},
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal surfaces for Docket",
}

var tuiWatchCmd = &cobra.Command{
	Use:   "watch [TKT-NNN]",
	Short: "Open the managed-run dashboard and launcher",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := ""
		if len(args) == 1 {
			ticketID = args[0]
		}
		if runWorkspace {
			if ticketID != "" {
				return fmt.Errorf("workspace watch does not support a ticket id argument yet")
			}
			return runWorkspaceWatchDashboard(repo)
		}
		if ticketID != "" {
			return runWatchDashboard(repo, ticketID, nil, false, nil)
		}
		return runWatchDashboard(repo, "", nil, false, runWatchLaunchOptions(repo))
	},
}

var tuiRunLogCmd = &cobra.Command{
	Use:   "run-log <TKT-NNN>",
	Short: "Show the visible managed-run transcript for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := runruntime.New(repo)
		if warnings, err := store.HealRuntimeState(time.Now()); err == nil && len(warnings) > 0 && format != "json" {
			for _, warning := range warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning)
			}
		}
		transcript, err := store.LoadTranscript(args[0])
		if err != nil {
			return fmt.Errorf("load transcript: %w", err)
		}
		if format == "json" {
			printJSON(cmd, map[string]any{
				"ticket_id":   args[0],
				"transcript":  transcript,
				"entry_count": len(transcript),
			})
			return nil
		}
		if len(transcript) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: no visible managed-run transcript\n", args[0])
			return nil
		}
		for _, entry := range transcript {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", formatTranscriptTimestampLocal(entry.At), entry.Text)
		}
		return nil
	},
}

func renderTicketRunSummary(cmd *cobra.Command, summary agentrun.TicketRunSummary) error {
	if format == "json" {
		printJSON(cmd, summary)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s", summary.TicketID, summary.Status)
	if summary.Reason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " (%s)", summary.Reason)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

func renderCycleSummary(cmd *cobra.Command, summary agentrun.CycleSummary) error {
	if format == "json" {
		printJSON(cmd, summary)
		return nil
	}
	for _, run := range summary.Runs {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s", run.TicketID, run.Status)
		if run.Reason != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", run.Reason)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	if summary.StopReason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Stopped: %s\n", summary.StopReason)
	}
	return nil
}

func renderPingSummary(cmd *cobra.Command, summary agentrun.PingSummary) error {
	if format == "json" {
		printJSON(cmd, summary)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s", summary.TicketID)
	if strings.TrimSpace(summary.SessionID) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " session=%s", summary.SessionID)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	if len(summary.Lines) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No structured ping response.")
		return nil
	}
	for _, line := range summary.Lines {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}

func isRecoverableManagedRunStatus(status runruntime.StatusSnapshot) bool {
	if strings.TrimSpace(status.SessionID) == "" {
		return false
	}
	if status.Hung {
		return true
	}
	switch strings.TrimSpace(status.LastResultStatus) {
	case string(agentrun.StatusStuck), string(agentrun.StatusFailed):
		return true
	default:
		return false
	}
}

func isOperatorStopReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "operator requested hard stop",
		"operator requested stop after current ticket",
		"operator requested stop before starting the next ticket":
		return true
	default:
		return false
	}
}

func executeTicketRun(cmd *cobra.Command, ticketID string, run func(context.Context) (agentrun.TicketRunSummary, error)) (agentrun.TicketRunSummary, error) {
	healManagedRuntime(repo)
	if !runWatch {
		stopLogs := startLiveRunLogs(cmd, repo)
		defer stopLogs()
		return run(context.Background())
	}
	return runTicketWithWatch(repo, ticketID, run)
}

func executeCycleRun(cmd *cobra.Command, run func(context.Context) (agentrun.CycleSummary, error)) (agentrun.CycleSummary, error) {
	healManagedRuntime(repo)
	if !runWatch {
		stopLogs := startLiveRunLogs(cmd, repo)
		defer stopLogs()
		return run(context.Background())
	}
	return runCycleWithWatch(repo, run)
}

func runTicketWithWatch(repoRoot, ticketID string, run func(context.Context) (agentrun.TicketRunSummary, error)) (agentrun.TicketRunSummary, error) {
	type watchResult struct {
		summary agentrun.TicketRunSummary
		err     error
	}
	doneCh := make(chan struct{})
	resultCh := make(chan watchResult, 1)
	go func() {
		defer close(doneCh)
		summary, err := run(context.Background())
		resultCh <- watchResult{summary: summary, err: err}
	}()
	if err := runWatchDashboard(repoRoot, ticketID, doneCh, true, nil); err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	res := <-resultCh
	return res.summary, res.err
}

func runCycleWithWatch(repoRoot string, run func(context.Context) (agentrun.CycleSummary, error)) (agentrun.CycleSummary, error) {
	type watchResult struct {
		summary agentrun.CycleSummary
		err     error
	}
	doneCh := make(chan struct{})
	resultCh := make(chan watchResult, 1)
	go func() {
		defer close(doneCh)
		summary, err := run(context.Background())
		resultCh <- watchResult{summary: summary, err: err}
	}()
	if err := runWatchDashboard(repoRoot, "", doneCh, true, nil); err != nil {
		return agentrun.CycleSummary{}, err
	}
	res := <-resultCh
	return res.summary, res.err
}

func runWatchDashboard(repoRoot, ticketID string, doneCh <-chan struct{}, quitOnDone bool, launchOptions []tui.RunWatchLaunchOption) error {
	model := tui.NewRunWatchModel(repoRoot, ticketID, doneCh, quitOnDone, launchOptions)
	program := tea.NewProgram(model, runWatchProgramOptions(runWatchMouse)...)
	_, err := program.Run()
	return err
}

func runWatchProgramOptions(enableMouse bool) []tea.ProgramOption {
	options := []tea.ProgramOption{tea.WithAltScreen()}
	if enableMouse {
		options = append(options, tea.WithMouseCellMotion())
	}
	return options
}

func runWorkspaceWatchDashboard(workspaceRoot string) error {
	repos, err := workspace.Discover(workspaceRoot)
	if err != nil {
		return fmt.Errorf("discovering workspace repos: %w", err)
	}
	options, initialRepo, err := workspaceRunWatchLaunchOptions(workspaceRoot, repos)
	if err != nil {
		return err
	}
	return runWatchDashboard(initialRepo, "", nil, false, options)
}

func runWatchLaunchOptions(repoRoot string) []tui.RunWatchLaunchOption {
	return []tui.RunWatchLaunchOption{
		{
			ID:          "single-session",
			Label:       "Start Next Ticket",
			Description: "Pick the next runnable ticket and run it in a persisted Codex session that can be resumed later.",
			Start: func() (string, error) {
				return launchManagedSingleRunWithMode(repoRoot, "session")
			},
		},
		{
			ID:          "auto-session",
			Label:       "Start Auto Cycle",
			Description: "Keep running tickets using persisted Codex sessions so follow-up resumes stay on the same thread.",
			Start: func() (string, error) {
				return launchManagedAutoCycleWithMode(repoRoot, "session")
			},
		},
		{
			ID:          "attach",
			Label:       "Attach To Active Run",
			Description: "Watch the current managed run without starting a new one.",
			Start:       nil,
		},
		{
			ID:          "ping",
			Label:       "Ping Active Session",
			Description: "Send a same-thread structured status prompt into the active persisted Codex session.",
			StayInMenu:  true,
			Start: func() (string, error) {
				ticketID, err := currentManagedRunTicketID(repoRoot)
				if err != nil {
					return "", err
				}
				if ticketID == "" {
					return "", fmt.Errorf("no active managed run to ping")
				}
				svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), "session")
				_, err = svc.PingTicket(context.Background(), ticketID)
				return "ping completed", err
			},
		},
		{
			ID:          "clean",
			Label:       "Clean Stale Runs",
			Description: "Remove inactive stale runtime records and clear invalid cycle state.",
			StayInMenu:  true,
			Start: func() (string, error) {
				store := runruntime.New(repoRoot)
				if _, err := store.HealRuntimeState(time.Now()); err != nil {
					return "", err
				}
				_, err := store.CleanupStaleRuns()
				return "stale runs cleaned", err
			},
		},
	}
}

func workspaceRunWatchLaunchOptions(workspaceRoot string, repos []workspace.Repo) ([]tui.RunWatchLaunchOption, string, error) {
	if len(repos) == 0 {
		return nil, "", fmt.Errorf("no connected Docket repos found under %s", workspaceRoot)
	}
	initialRepo := repos[0].Path
	options := make([]tui.RunWatchLaunchOption, 0, len(repos)*5)
	for _, repoItem := range repos {
		labelSuffix := " • " + repoItem.Name
		descSuffix := relativeRepoLabel(workspaceRoot, repoItem.Path)
		if current, err := currentManagedRunTicketID(repoItem.Path); err == nil && strings.TrimSpace(current) != "" {
			initialRepo = repoItem.Path
			options = append(options, tui.RunWatchLaunchOption{
				ID:          "attach:" + repoItem.Name,
				Label:       "Attach To Active Run" + labelSuffix,
				Description: "Watch the current managed run in " + descSuffix + ".",
				RepoRoot:    repoItem.Path,
				Start:       nil,
			})
		}
		options = append(options,
			tui.RunWatchLaunchOption{
				ID:          "single-session:" + repoItem.Name,
				Label:       "Start Next Ticket" + labelSuffix,
				Description: "Pick the next runnable ticket in " + descSuffix + " and run it in a persisted Codex session.",
				RepoRoot:    repoItem.Path,
				Start: func(repoRoot string) func() (string, error) {
					return func() (string, error) { return launchManagedSingleRunWithMode(repoRoot, "session") }
				}(repoItem.Path),
			},
			tui.RunWatchLaunchOption{
				ID:          "auto-session:" + repoItem.Name,
				Label:       "Start Auto Cycle" + labelSuffix,
				Description: "Keep running runnable tickets in " + descSuffix + " using persisted Codex sessions.",
				RepoRoot:    repoItem.Path,
				Start: func(repoRoot string) func() (string, error) {
					return func() (string, error) { return launchManagedAutoCycleWithMode(repoRoot, "session") }
				}(repoItem.Path),
			},
			tui.RunWatchLaunchOption{
				ID:          "ping:" + repoItem.Name,
				Label:       "Ping Active Session" + labelSuffix,
				Description: "Send a structured status ping into the active run for " + descSuffix + ".",
				RepoRoot:    repoItem.Path,
				StayInMenu:  true,
				Start: func(repoRoot string) func() (string, error) {
					return func() (string, error) {
						ticketID, err := currentManagedRunTicketID(repoRoot)
						if err != nil {
							return "", err
						}
						if ticketID == "" {
							return "", fmt.Errorf("no active managed run to ping")
						}
						svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), "session")
						_, err = svc.PingTicket(context.Background(), ticketID)
						return "ping completed", err
					}
				}(repoItem.Path),
			},
			tui.RunWatchLaunchOption{
				ID:          "clean:" + repoItem.Name,
				Label:       "Clean Stale Runs" + labelSuffix,
				Description: "Remove inactive stale runtime records in " + descSuffix + ".",
				RepoRoot:    repoItem.Path,
				StayInMenu:  true,
				Start: func(repoRoot string) func() (string, error) {
					return func() (string, error) {
						store := runruntime.New(repoRoot)
						if _, err := store.HealRuntimeState(time.Now()); err != nil {
							return "", err
						}
						_, err := store.CleanupStaleRuns()
						return "stale runs cleaned", err
					}
				}(repoItem.Path),
			},
		)
	}
	return options, initialRepo, nil
}

func relativeRepoLabel(workspaceRoot, repoRoot string) string {
	rel, err := filepath.Rel(workspaceRoot, repoRoot)
	if err != nil || strings.TrimSpace(rel) == "" {
		return filepath.Base(repoRoot)
	}
	return rel
}

func launchManagedSingleRun(repoRoot string) (string, error) {
	return launchManagedSingleRunWithMode(repoRoot, managedRunAdapterMode())
}

func launchManagedSingleRunWithMode(repoRoot, mode string) (string, error) {
	ctx := context.Background()
	healManagedRuntime(repoRoot)
	store := local.New(repoRoot)
	if err := store.SyncIndex(ctx); err != nil {
		return "", fmt.Errorf("syncing index: %w", err)
	}
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return "", err
	}
	next, err := selectNextTicket(ctx, store, cfg)
	if err != nil {
		return "", err
	}
	if next == nil {
		diagnosis, err := workablepkg.DiagnoseEmpty(ctx, store, cfg)
		if err != nil {
			return "", err
		}
		return diagnosis.Summary(), nil
	}
	svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), mode)
	summary, err := svc.RunTicket(ctx, next.ID)
	if err != nil {
		return "", err
	}
	if err := singleRunSummaryError(summary); err != nil {
		return "", err
	}
	if isOperatorStopReason(summary.Reason) {
		return summary.Reason, nil
	}
	return fmt.Sprintf("%s finished", summary.TicketID), nil
}

func launchManagedAutoCycle(repoRoot string) (string, error) {
	return launchManagedAutoCycleWithMode(repoRoot, managedRunAdapterMode())
}

func launchManagedAutoCycleWithMode(repoRoot, mode string) (string, error) {
	healManagedRuntime(repoRoot)
	svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), mode)
	s := local.New(repoRoot)
	ctx := context.Background()
	for {
		summary, err := svc.RunNext(ctx)
		if err != nil {
			return "", err
		}
		if err := cycleSummaryError(summary); err != nil {
			return "", err
		}
		reason := strings.TrimSpace(summary.StopReason)
		switch reason {
		case "":
		case "operator requested stop after current ticket", "operator requested stop before starting the next ticket":
			return reason, nil
		default:
			if !isNoRunnableReason(reason) {
				return "cycle finished", nil
			}
		}

		if err := s.SyncIndex(ctx); err != nil {
			return "", fmt.Errorf("syncing index: %w", err)
		}
		cfg, err := ticket.LoadConfig(repoRoot)
		if err != nil {
			return "", err
		}
		pending, err := countStartableTickets(ctx, s, cfg)
		if err != nil {
			return "", err
		}
		if pending == 0 {
			if reason == "" {
				return "out of tickets", nil
			}
			return reason, nil
		}

		resume, err := selectResumableActiveTicket(ctx, s, cfg)
		if err != nil {
			return "", err
		}
		if resume != nil {
			runSummary, err := svc.RunTicket(ctx, resume.ID)
			if err != nil {
				return "", err
			}
			if err := singleRunSummaryError(runSummary); err != nil {
				return "", err
			}
			if isOperatorStopReason(runSummary.Reason) {
				return runSummary.Reason, nil
			}
			continue
		}

		time.Sleep(runWatchRetryDelay)
	}
}

func countStartableTickets(ctx context.Context, s *local.Store, cfg *ticket.Config) (int, error) {
	if cfg == nil {
		return 0, nil
	}
	startable := cfg.StartableStates()
	if len(startable) == 0 {
		return 0, nil
	}
	filter := store.Filter{States: make([]ticket.State, 0, len(startable))}
	for _, state := range startable {
		filter.States = append(filter.States, ticket.State(state))
	}
	tickets, err := s.ListTickets(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(tickets), nil
}

func singleRunSummaryError(summary agentrun.TicketRunSummary) error {
	if summary.Status == agentrun.StatusDone {
		return nil
	}
	reason := strings.TrimSpace(summary.Reason)
	if isOperatorStopReason(reason) {
		return nil
	}
	if reason == "" {
		reason = string(summary.Status)
	}
	if strings.TrimSpace(summary.TicketID) == "" {
		return fmt.Errorf("managed run failed: %s", reason)
	}
	return fmt.Errorf("%s failed: %s", summary.TicketID, reason)
}

func cycleSummaryError(summary agentrun.CycleSummary) error {
	for _, run := range summary.Runs {
		if err := singleRunSummaryError(run); err != nil {
			return err
		}
	}
	if len(summary.Runs) == 0 {
		reason := strings.TrimSpace(summary.StopReason)
		if reason == "" || isNoRunnableReason(reason) || isOperatorStopReason(reason) {
			return nil
		}
		return fmt.Errorf("managed run stopped: %s", reason)
	}
	return nil
}

func isNoRunnableReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	return reason == "no runnable tickets remain" || strings.HasPrefix(reason, "no runnable tickets remain:")
}

func renderRunBriefHuman(cmd *cobra.Command, brief runruntime.RunBrief) {
	if strings.TrimSpace(brief.Outcome) == "" {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Last outcome: %s\n", brief.Outcome)
	if strings.TrimSpace(brief.TicketID) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Ticket: %s\n", brief.TicketID)
	}
	if strings.TrimSpace(brief.Summary) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %s\n", brief.Summary)
	}
	if strings.TrimSpace(brief.CommitSHA) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Commit: %s\n", brief.CommitSHA)
	}
	if strings.TrimSpace(brief.CloseoutCommitSHA) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Closeout commit: %s\n", brief.CloseoutCommitSHA)
	}
	if strings.TrimSpace(brief.Tests) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Validation: %s\n", brief.Tests)
	}
	if len(brief.ValidationErrors) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Validation errors: %s\n", strings.Join(brief.ValidationErrors, "; "))
	}
	if strings.TrimSpace(brief.ResumeNext) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Resume next: %s\n", brief.ResumeNext)
	}
}

func renderRecoverableBriefHintHuman(cmd *cobra.Command, ticketID string, status runruntime.StatusSnapshot) {
	if strings.TrimSpace(status.SessionID) != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Session: %s\n", status.SessionID)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Recoverable: true")
	fmt.Fprintf(cmd.OutOrStdout(), "Resume with: docket run-resume %s\n", ticketID)
}

func healManagedRuntime(repoRoot string) {
	store := runruntime.New(repoRoot)
	_, _ = store.HealRuntimeState(time.Now())
}

func currentManagedRunTicketID(repoRoot string) (string, error) {
	store := runruntime.New(repoRoot)
	if cycle, ok, err := store.LoadCycleState(); err != nil {
		return "", err
	} else if ok && strings.TrimSpace(cycle.CurrentTicketID) != "" {
		return cycle.CurrentTicketID, nil
	}
	ticketIDs, err := store.ListRunTicketIDs()
	if err != nil {
		return "", err
	}
	sort.Strings(ticketIDs)
	for _, ticketID := range ticketIDs {
		status, ok, err := store.LoadStatus(ticketID)
		if err != nil || !ok {
			continue
		}
		if status.Active || status.Hung {
			return ticketID, nil
		}
	}
	return "", nil
}

func init() {
	runTicketCmd.Flags().BoolVar(&runEnableReview, "review", false, "run the optional reviewer pass and capped fix loop after implementer validation")
	runNextCmd.Flags().BoolVar(&runEnableReview, "review", false, "run the optional reviewer pass and capped fix loop after implementer validation")
	runTicketCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "compatibility alias; reviewer pass is disabled by default")
	runNextCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "compatibility alias; reviewer pass is disabled by default")
	runTicketCmd.Flags().BoolVar(&runWatch, "watch", false, "open the interactive managed-run dashboard while this run is active")
	runTicketCmd.Flags().BoolVar(&runWatchMouse, "watch-mouse", false, "enable mouse capture in the managed-run dashboard")
	runTicketCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode")
	runNextCmd.Flags().BoolVar(&runWatch, "watch", false, "open the interactive managed-run dashboard while this run is active")
	runNextCmd.Flags().BoolVar(&runWatchMouse, "watch-mouse", false, "enable mouse capture in the managed-run dashboard")
	runNextCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode")
	runTicketCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "run a managed-run health check after this much time without new Codex output")
	runNextCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "run a managed-run health check after this much time without new Codex output")
	runResumeCmd.Flags().BoolVar(&runEnableReview, "review", false, "run the optional reviewer pass and capped fix loop after implementer validation")
	runResumeCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "compatibility alias; reviewer pass is disabled by default")
	runResumeCmd.Flags().BoolVar(&runWatch, "watch", false, "open the interactive managed-run dashboard while this run is active")
	runResumeCmd.Flags().BoolVar(&runWatchMouse, "watch-mouse", false, "enable mouse capture in the managed-run dashboard")
	runResumeCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode")
	runResumeCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "run a managed-run health check after this much time without new Codex output")
	runPingCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode")
	rootCmd.AddCommand(runTicketCmd)
	rootCmd.AddCommand(runNextCmd)
	rootCmd.AddCommand(runStatusCmd)
	rootCmd.AddCommand(runResumeCmd)
	rootCmd.AddCommand(runPingCmd)
	rootCmd.AddCommand(runWatchCmd)
	runWatchCmd.Flags().BoolVar(&runWatchMouse, "mouse", false, "enable mouse capture in the managed-run dashboard")
	runWatchCmd.Flags().BoolVar(&runWorkspace, "workspace", false, "aggregate connected Docket repos under the current workspace root")
	tuiCmd.AddCommand(tuiWatchCmd)
	tuiWatchCmd.Flags().BoolVar(&runWatchMouse, "mouse", false, "enable mouse capture in the managed-run dashboard")
	tuiWatchCmd.Flags().BoolVar(&runWorkspace, "workspace", false, "aggregate connected Docket repos under the current workspace root")
	tuiCmd.AddCommand(tuiRunLogCmd)
	rootCmd.AddCommand(tuiCmd)
}
