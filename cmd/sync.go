package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Rebuild the ticket index cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)
		ctx := context.Background()

		if err := s.SyncIndex(ctx); err != nil {
			return fmt.Errorf("syncing index: %w", err)
		}

		// Count tickets in the index
		tickets, err := s.ListTickets(ctx, store.Filter{IncludeArchived: true})
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Synced index: %d tickets.\n", len(tickets))

		// Reconcile any direct edits
		results, err := s.ReconcileTampering(ctx)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not reconcile direct edits: %v\n", err)
			return nil
		}

		if len(results) == 0 {
			return nil
		}

		accepted := 0
		rejected := 0
		for _, r := range results {
			if r.Accepted {
				accepted++
				// Build a hint about what changed
				fields := make([]string, 0, len(r.Changes))
				for _, ch := range r.Changes {
					fields = append(fields, ch.Field)
				}
				hint := prescriptiveCommand(r.ID, r.Changes[0].Field, r.Changes[0].Actual)
				fmt.Fprintf(cmd.OutOrStdout(), "  accepted (valid): %s — direct edit detected (fields: %s). Next time use: %s\n",
					r.ID, strings.Join(fields, ", "), hint)
			} else {
				rejected++
				if r.Reverted {
					fmt.Fprintf(cmd.ErrOrStderr(), "  rejected (invalid) and reverted: %s — direct edit had schema errors:\n", r.ID)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "  rejected (invalid): %s — direct edit has schema errors:\n", r.ID)
				}
				for _, e := range r.Errors {
					fmt.Fprintf(cmd.ErrOrStderr(), "    - %s: %s\n", e.Field, e.Message)
				}
				if r.Reverted {
					fmt.Fprintf(cmd.ErrOrStderr(), "  The file was reverted to the last valid values. Use CLI commands for changes.\n")
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "  Fix with CLI instead of editing the file directly.\n")
				}
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Reconciled %d direct edits: %d accepted, %d rejected.\n",
			len(results), accepted, rejected)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
