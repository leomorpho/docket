package cmd

import (
	"context"
	"fmt"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var acListCmd = &cobra.Command{
	Use:   "list <TKT-NNN>",
	Short: "List acceptance criteria for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		if format == "json" {
			printJSON(cmd, map[string]interface{}{"ticket_id": id, "ac": t.AC})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Acceptance criteria for %s:\n", id)
		for i, ac := range t.AC {
			mark := "[ ]"
			if ac.Done {
				mark = "[x]"
			}
			line := fmt.Sprintf("%d. %s %s", i+1, mark, ac.Description)
			if ac.Run != "" {
				line += " (run: " + ac.Run + ")"
			}
			if ac.Evidence != "" {
				line += " — evidence: " + ac.Evidence
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		return nil
	},
}

func init() {
	acCmd.AddCommand(acListCmd)
}
