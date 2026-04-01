package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var readyCmd = &cobra.Command{
	Use:          "ready <TKT-NNN>",
	Short:        "Check whether a ticket satisfies the ready contract",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		if normalized, ok := ticket.NormalizeID(id); ok {
			id = normalized
		}

		report, err := local.New(repo).CheckReady(context.Background(), id)
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, report)
			if !report.Ready {
				return fmt.Errorf("ready contract failed for %s", report.TicketID)
			}
			return nil
		}

		renderReadyCheckHuman(cmd, report)
		if !report.Ready {
			return fmt.Errorf("ready contract failed for %s", report.TicketID)
		}
		return nil
	},
}

func renderReadyCheckHuman(cmd *cobra.Command, report local.ReadyCheckResult) {
	if report.Ready {
		fmt.Fprintf(cmd.OutOrStdout(), "%s passes ready contract.\n", report.TicketID)
		fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", report.State)
		return
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s fails ready contract.\n", report.TicketID)
	fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", report.State)
	for _, issue := range report.Issues {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s: %s\n", issue.Field, issue.Message)
	}
}

func init() {
	rootCmd.AddCommand(readyCmd)
}
