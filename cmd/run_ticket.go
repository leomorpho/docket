package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

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
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
	"github.com/spf13/cobra"
)

const DefaultRunInactivityTimeout = 10 * time.Minute

var (
	runWithReview      bool
	runInactivityLimit time.Duration
)

var newRunOrchestrator = func(repoRoot string, enableReview bool) agentrun.Orchestrator {
	store := local.New(repoRoot)
	wf := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), newRuntimeDeps(repoRoot).claimer)
	runtimeStore := runruntime.New(repoRoot)
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
		Adapter:   codex.NewRunner(),
		Monitor:   monitor.New(monitor.Dependencies{Runtime: runtimeStore}),
		Validator: validator,
		Selector:  selector.New(selector.Dependencies{Store: store, LoadConfig: ticket.LoadConfig}),
		Runtime:   runtimeStore,
		Timeout:   runInactivityLimitOrDefault(),
	}
	if enableReview {
		deps.Reviewer = codex.NewRunner()
	}
	return orchestrate.New(deps)
}

func runActor() string {
	actor := detectActor()
	if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
		actor = "agent:" + agentID
	}
	return actor
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
		svc := newRunOrchestrator(repo, runWithReview)
		summary, err := svc.RunTicket(context.Background(), args[0])
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
		svc := newRunOrchestrator(repo, runWithReview)
		summary, err := svc.RunNext(context.Background())
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
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	},
}

var runResumeCmd = &cobra.Command{
	Use:   "run-resume <TKT-NNN>",
	Short: "Resume a hung ticket run in a fresh Codex session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newRunOrchestrator(repo, runWithReview)
		summary, err := svc.ResumeTicket(context.Background(), args[0])
		if err != nil {
			return err
		}
		return renderTicketRunSummary(cmd, summary)
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

func init() {
	runTicketCmd.Flags().BoolVar(&runWithReview, "review", false, "run one optional reviewer pass with a single capped fix-review loop")
	runNextCmd.Flags().BoolVar(&runWithReview, "review", false, "run one optional reviewer pass with a single capped fix-review loop")
	runTicketCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "mark the run hung after this much time without new Codex output")
	runNextCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "mark the run hung after this much time without new Codex output")
	runResumeCmd.Flags().BoolVar(&runWithReview, "review", false, "run one optional reviewer pass with a single capped fix-review loop")
	runResumeCmd.Flags().DurationVar(&runInactivityLimit, "inactivity-timeout", DefaultRunInactivityTimeout, "mark the resumed run hung after this much time without new Codex output")
	rootCmd.AddCommand(runTicketCmd)
	rootCmd.AddCommand(runNextCmd)
	rootCmd.AddCommand(runStatusCmd)
	rootCmd.AddCommand(runResumeCmd)
}
