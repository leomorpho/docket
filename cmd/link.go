package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var linkRelation string

var linkCmd = &cobra.Command{
	Use:   "link <TKT-X> <TKT-Y>",
	Short: "Link tickets with a relation",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beforeComponents, err := currentComponentCount(context.Background(), ticketRepoRoot(repo))
		if err != nil {
			return fmt.Errorf("checking graph health before link: %w", err)
		}
		previousState, err := loadRelations(ticketRepoRoot(repo))
		if err != nil {
			return fmt.Errorf("loading relations: %w", err)
		}
		switch linkRelation {
		case "blocks", "parallel-safe", "depends-on":
		default:
			return fmt.Errorf("--relation must be one of: blocks, parallel-safe, depends-on")
		}
		if err := upsertRelation(repo, relationEntry{
			From:     args[0],
			To:       args[1],
			Relation: linkRelation,
		}); err != nil {
			return err
		}
		if err := enforceMutationConnectivity(context.Background(), ticketRepoRoot(repo), beforeComponents); err != nil {
			if rollbackErr := saveRelations(ticketRepoRoot(repo), previousState); rollbackErr != nil {
				return fmt.Errorf("%v; rollback failed: %w", err, rollbackErr)
			}
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Linked %s -> %s (%s)\n", args[0], args[1], linkRelation)
		return nil
	},
}

func init() {
	linkCmd.Flags().StringVar(&linkRelation, "relation", "", "relation type: blocks|parallel-safe|depends-on")
	rootCmd.AddCommand(linkCmd)
}
