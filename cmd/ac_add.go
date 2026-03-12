package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var acAddDesc string
var acAddRun string

var acAddCmd = &cobra.Command{
	Use:   "add <TKT-NNN>",
	Short: "Add an acceptance criterion to a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(acAddDesc) == "" {
			return fmt.Errorf("--desc is required")
		}
		id := args[0]
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		t.AC = append(t.AC, ticket.AcceptanceCriterion{
			Description: strings.TrimSpace(acAddDesc),
			Done:        false,
			Run:         strings.TrimSpace(acAddRun),
		})
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := s.UpdateTicket(context.Background(), t); err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]interface{}{"ticket_id": id, "total": len(t.AC)})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Added AC to %s.\n", id)
		}
		return nil
	},
}

func init() {
	acAddCmd.Flags().StringVar(&acAddDesc, "desc", "", "acceptance criterion description")
	acAddCmd.Flags().StringVar(&acAddRun, "run", "", "optional command to execute during ac check")
	acCmd.AddCommand(acAddCmd)
}
