package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var errACIncomplete = errors.New("acceptance criteria incomplete")

var acCheckCmd = &cobra.Command{
	Use:          "check <TKT-NNN>",
	Short:        "Check whether all acceptance criteria are complete",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
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

		total := len(t.AC)
		done := 0
		var remaining []string
		for _, ac := range t.AC {
			if ac.Done {
				done++
			} else {
				remaining = append(remaining, ac.Description)
			}
		}
		complete := len(remaining) == 0

		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket_id": id,
				"complete":  complete,
				"total":     total,
				"done":      done,
				"remaining": remaining,
			})
		} else if complete {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s: all %d acceptance criteria met.\n", id, total)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "✗ %s: %d of %d acceptance criteria incomplete:\n", id, len(remaining), total)
			for _, r := range remaining {
				fmt.Fprintf(cmd.OutOrStdout(), "  [ ] %s\n", r)
			}
		}

		if !complete {
			return errACIncomplete
		}
		return nil
	},
}

func init() {
	acCmd.AddCommand(acCheckCmd)
}
