package cmd

import (
	"fmt"
	"strings"
	"time"

	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/spf13/cobra"
)

var runCleanupApply bool
var runCleanupDryRun bool

type runCleanupReport struct {
	Mode          string                           `json:"mode"`
	Applied       bool                             `json:"applied"`
	MutationCount int                              `json:"mutation_count"`
	Issues        []runruntime.ReconciliationIssue `json:"issues,omitempty"`
}

var runCleanupCmd = &cobra.Command{
	Use:   "run-cleanup",
	Short: "Scan repo-local runtime artifacts and report stale or missing managed-run state",
	RunE: func(cmd *cobra.Command, args []string) error {
		if runCleanupApply && runCleanupDryRun {
			return fmt.Errorf("choose either --apply or --dry-run")
		}

		report, err := buildRunCleanupReport(repo, runCleanupApply, time.Now().UTC())
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, report)
			return nil
		}
		renderRunCleanupHuman(cmd, report)
		return nil
	},
}

func buildRunCleanupReport(repoRoot string, apply bool, now time.Time) (runCleanupReport, error) {
	runtimeStore := runruntime.New(repoRoot)
	namespaceStore := runstate.New(runtimeNamespaceRoot(repoRoot))
	issues, err := runtimeStore.ScanReconciliationIssues(namespaceStore, now)
	if err != nil {
		return runCleanupReport{}, err
	}

	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	return runCleanupReport{
		Mode:          mode,
		Applied:       false,
		MutationCount: 0,
		Issues:        issues,
	}, nil
}

func renderRunCleanupHuman(cmd *cobra.Command, report runCleanupReport) {
	fmt.Fprintf(cmd.OutOrStdout(), "Runtime cleanup %s.\n", report.Mode)
	if len(report.Issues) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No runtime artifact issues found.")
	} else {
		for _, issue := range report.Issues {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s", runCleanupIssueTicket(issue), runCleanupIssueLabel(issue.Kind))
			if strings.TrimSpace(issue.LegacyState) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", issue.LegacyState)
			}
			if strings.TrimSpace(issue.Detail) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " - %s", issue.Detail)
			}
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	if report.MutationCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No mutations applied.")
	}
	if report.Mode != "apply" {
		fmt.Fprintln(cmd.OutOrStdout(), "Apply with: docket run-cleanup --apply")
	}
}

func runCleanupIssueTicket(issue runruntime.ReconciliationIssue) string {
	if strings.TrimSpace(issue.TicketID) != "" {
		return issue.TicketID
	}
	return "runtime"
}

func runCleanupIssueLabel(kind string) string {
	switch kind {
	case "orphan_run_dir":
		return "orphan run dir"
	case "stale_recoverable_status":
		return "stale recoverable status"
	case "missing_brief":
		return "missing durable brief"
	case "legacy_checkpoint":
		return "legacy checkpoint"
	default:
		return strings.ReplaceAll(strings.TrimSpace(kind), "_", " ")
	}
}

func init() {
	runCleanupCmd.Flags().BoolVar(&runCleanupApply, "apply", false, "scan runtime artifacts in preparation for cleanup repair")
	runCleanupCmd.Flags().BoolVar(&runCleanupDryRun, "dry-run", false, "report runtime artifact issues without mutating files")
	rootCmd.AddCommand(runCleanupCmd)
}
