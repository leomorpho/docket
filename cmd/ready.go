package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var readyPromote bool

var readyCmd = &cobra.Command{
	Use:          "ready <TKT-NNN>",
	Short:        "Check whether a ticket satisfies the ready contract",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) (runErr error) {
		defer func() {
			readyPromote = false
		}()
		defer func() {
			runErr = renderMutationError(cmd, runErr)
		}()

		id := strings.TrimSpace(args[0])
		if normalized, ok := ticket.NormalizeID(id); ok {
			id = normalized
		}

		store := local.New(repo)
		if readyPromote {
			report, promoted, err := store.PromoteReady(context.Background(), id)
			if err != nil {
				if report.TicketID != "" {
					if format == "json" {
						printJSON(cmd, report)
					} else {
						renderReadyCheckHuman(cmd, report)
					}
				}
				return err
			}
			if format == "json" {
				printJSON(cmd, map[string]any{
					"ticket_id": report.TicketID,
					"ready":     report.Ready,
					"state":     report.State,
					"promoted":  promoted,
				})
				return nil
			}
			if promoted {
				fmt.Fprintf(cmd.OutOrStdout(), "%s promoted to ready.\n", report.TicketID)
				fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", report.State)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is already ready.\n", report.TicketID)
			fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", report.State)
			return nil
		}

		report, err := store.CheckReady(context.Background(), id)
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
	readyCmd.Flags().BoolVar(&readyPromote, "promote", false, "promote a draft leaf ticket to ready when the ready contract passes")
	rootCmd.AddCommand(readyCmd)
}
