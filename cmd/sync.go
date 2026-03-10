package cmd

import (
	"context"
	"fmt"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/store/local"
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
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
