package cmd

import (
	"context"
	"errors"
	"fmt"

	ck "github.com/leoaudibert/docket/internal/check"
	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var checkFix bool

var errCheckFindings = errors.New("check findings present")

var checkCmd = &cobra.Command{
	Use:   "check [TKT-NNN]",
	Short: "Run staleness and consistency checks",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s := local.New(repo)
		ctx := context.Background()

		tickets, err := loadTicketsForCheck(ctx, s, args)
		if err != nil {
			return err
		}

		checker := ck.NewChecker(s)
		findings, err := checker.Run(ctx, tickets, checkFix)
		if err != nil {
			return err
		}

		errorsCount := 0
		warnCount := 0
		for _, f := range findings {
			if f.Severity == ck.SeverityError {
				errorsCount++
			} else {
				warnCount++
			}
		}

		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"checked":  len(tickets),
				"findings": findings,
				"summary":  map[string]int{"errors": errorsCount, "warnings": warnCount},
			})
		} else if len(findings) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "All %d tickets look healthy.\n", len(tickets))
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "docket check — %d tickets checked\n\n", len(tickets))
			for _, f := range findings {
				marker := "⚠"
				if f.Severity == ck.SeverityError {
					marker = "✗"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s %s: %s\n", f.TicketID, marker, f.Rule, f.Message)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d findings: %d errors, %d warnings.\n", len(findings), errorsCount, warnCount)
		}

		if len(findings) > 0 && !checkFix {
			return errCheckFindings
		}
		return nil
	},
}

func loadTicketsForCheck(ctx context.Context, s *local.Store, args []string) ([]*ticket.Ticket, error) {
	if len(args) == 1 {
		t, err := s.GetTicket(ctx, args[0])
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, fmt.Errorf("ticket %s not found", args[0])
		}
		return []*ticket.Ticket{t}, nil
	}
	return s.ListTickets(ctx, store.Filter{States: []ticket.State{ticket.StateBacklog, ticket.StateTodo, ticket.StateInProgress, ticket.StateInReview, ticket.StateDone}})
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "apply low-risk automatic fixes")
	rootCmd.AddCommand(checkCmd)
}
