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
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/tui"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/leomorpho/docket/internal/workspace"
	"github.com/spf13/cobra"
)

const DefaultRunInactivityTimeout = 2 * time.Minute

var (
	runDisableReview   bool
	runInactivityLimit time.Duration
	runManagedAdapter  string
	runWatch           bool
	runWatchMouse      bool
	runWorkspace       bool
)

var newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
	return newRunOrchestratorWithMode(repoRoot, enableReview, managedRunAdapterMode())
}

var newRunOrchestratorWithMode = func(repoRoot string, enableReview bool, mode string) agentrun.Orchestrator {
	store := local.New(repoRoot)
	wf := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), newRuntimeDeps(repoRoot).claimer)
	runtimeStore := runruntime.New(repoRoot)
	adapter := managedRunAdapter(mode)
	validator := runvalidate.New(runvalidate.Dependencies{
		RepoRoot: repoRoot,
		Store:    store,
		Workflow: wf,
	})
	deps := orchestrate.Dependencies{
		RepoRoot:  repoRoot,
		Actor:     runActor(),
		Store:     store,
		Workflow:  wf,
		Namespace: security.NewRepoNamespaceStore(docketHome),
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
		writeTerminalTitle(s.cmd, formatManagedRunTitle(filepathBase(repo), ticketID, status.CurrentPhase, status.CurrentStep, status.PlannedSteps, status.Active))
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
	return !runDisableReview
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
		status, ok, err := store.LoadStatus(args[0])
		if err != nil {
			return err
		}
		if !ok {
			if format == "json" {
				printJSON(cmd, map[string]any{"ticket_id": args[0], "active": false})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: no active run\n", args[0])
			return nil
		}
		if format == "json" {
			printJSON(cmd, status)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: active=%t hung=%t", status.TicketID, status.Active, status.Hung)
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
		fmt.Fprintln(cmd.OutOrStdout())
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
			Start: func() error {
				return launchManagedSingleRunWithMode(repoRoot, "session")
			},
		},
		{
			ID:          "auto-session",
			Label:       "Start Auto Cycle",
			Description: "Keep running tickets using persisted Codex sessions so follow-up resumes stay on the same thread.",
			Start: func() error {
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
			Start: func() error {
				ticketID, err := currentManagedRunTicketID(repoRoot)
				if err != nil {
					return err
				}
				if ticketID == "" {
					return fmt.Errorf("no active managed run to ping")
				}
				svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), "session")
				_, err = svc.PingTicket(context.Background(), ticketID)
				return err
			},
		},
		{
			ID:          "clean",
			Label:       "Clean Stale Runs",
			Description: "Remove inactive stale runtime records and clear invalid cycle state.",
			StayInMenu:  true,
			Start: func() error {
				store := runruntime.New(repoRoot)
				if _, err := store.HealRuntimeState(time.Now()); err != nil {
					return err
				}
				_, err := store.CleanupStaleRuns()
				return err
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
				Start: func(repoRoot string) func() error {
					return func() error { return launchManagedSingleRunWithMode(repoRoot, "session") }
				}(repoItem.Path),
			},
			tui.RunWatchLaunchOption{
				ID:          "auto-session:" + repoItem.Name,
				Label:       "Start Auto Cycle" + labelSuffix,
				Description: "Keep running runnable tickets in " + descSuffix + " using persisted Codex sessions.",
				RepoRoot:    repoItem.Path,
				Start: func(repoRoot string) func() error {
					return func() error { return launchManagedAutoCycleWithMode(repoRoot, "session") }
				}(repoItem.Path),
			},
			tui.RunWatchLaunchOption{
				ID:          "ping:" + repoItem.Name,
				Label:       "Ping Active Session" + labelSuffix,
				Description: "Send a structured status ping into the active run for " + descSuffix + ".",
				RepoRoot:    repoItem.Path,
				StayInMenu:  true,
				Start: func(repoRoot string) func() error {
					return func() error {
						ticketID, err := currentManagedRunTicketID(repoRoot)
						if err != nil {
							return err
						}
						if ticketID == "" {
							return fmt.Errorf("no active managed run to ping")
						}
						svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), "session")
						_, err = svc.PingTicket(context.Background(), ticketID)
						return err
					}
				}(repoItem.Path),
			},
			tui.RunWatchLaunchOption{
				ID:          "clean:" + repoItem.Name,
				Label:       "Clean Stale Runs" + labelSuffix,
				Description: "Remove inactive stale runtime records in " + descSuffix + ".",
				RepoRoot:    repoItem.Path,
				StayInMenu:  true,
				Start: func(repoRoot string) func() error {
					return func() error {
						store := runruntime.New(repoRoot)
						if _, err := store.HealRuntimeState(time.Now()); err != nil {
							return err
						}
						_, err := store.CleanupStaleRuns()
						return err
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

func launchManagedSingleRun(repoRoot string) error {
	return launchManagedSingleRunWithMode(repoRoot, managedRunAdapterMode())
}

func launchManagedSingleRunWithMode(repoRoot, mode string) error {
	ctx := context.Background()
	healManagedRuntime(repoRoot)
	store := local.New(repoRoot)
	if err := store.SyncIndex(ctx); err != nil {
		return fmt.Errorf("syncing index: %w", err)
	}
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return err
	}
	next, err := selectNextTicket(ctx, store, cfg)
	if err != nil {
		return err
	}
	if next == nil {
		return fmt.Errorf("no runnable tickets remain")
	}
	svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), mode)
	summary, err := svc.RunTicket(ctx, next.ID)
	if err != nil {
		return err
	}
	return singleRunSummaryError(summary)
}

func launchManagedAutoCycle(repoRoot string) error {
	return launchManagedAutoCycleWithMode(repoRoot, managedRunAdapterMode())
}

func launchManagedAutoCycleWithMode(repoRoot, mode string) error {
	healManagedRuntime(repoRoot)
	svc := newRunOrchestratorWithMode(repoRoot, runReviewEnabled(), mode)
	summary, err := svc.RunNext(context.Background())
	if err != nil {
		return err
	}
	return cycleSummaryError(summary)
}

func singleRunSummaryError(summary agentrun.TicketRunSummary) error {
	if summary.Status == agentrun.StatusDone {
		return nil
	}
	reason := strings.TrimSpace(summary.Reason)
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
		if reason == "" || reason == "no runnable tickets remain" {
			return nil
		}
		return fmt.Errorf("managed run stopped: %s", reason)
	}
	return nil
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
	runTicketCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "skip the default reviewer pass and capped fix-review loop")
	runNextCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "skip the default reviewer pass and capped fix-review loop")
	runTicketCmd.Flags().BoolVar(&runWatch, "watch", false, "open the interactive managed-run dashboard while this run is active")
	runTicketCmd.Flags().BoolVar(&runWatchMouse, "watch-mouse", false, "enable mouse capture in the managed-run dashboard")
	runTicketCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode")
	runNextCmd.Flags().BoolVar(&runWatch, "watch", false, "open the interactive managed-run dashboard while this run is active")
	runNextCmd.Flags().BoolVar(&runWatchMouse, "watch-mouse", false, "enable mouse capture in the managed-run dashboard")
	runNextCmd.Flags().StringVar(&runManagedAdapter, "managed-run-adapter", "session", "managed run adapter mode")
	runTicketCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "run a managed-run health check after this much time without new Codex output")
	runNextCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "run a managed-run health check after this much time without new Codex output")
	runResumeCmd.Flags().BoolVar(&runDisableReview, "no-review", false, "skip the default reviewer pass and capped fix-review loop")
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
