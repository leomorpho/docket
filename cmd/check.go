package cmd

import (
	"context"
	"errors"
	"fmt"

	ck "github.com/leomorpho/docket/internal/check"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	checkFix    bool
	checkDoctor bool
)

var errCheckFindings = errors.New("check findings present")

var checkCmd = &cobra.Command{
	Use:          "check [TKT-NNN]",
	Short:        "Run staleness and consistency checks",
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
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

		if checkDoctor {
			doctorFindings, _ := runDoctor(ctx, s)
			findings = append(findings, doctorFindings...)
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

func runDoctor(ctx context.Context, s *local.Store) ([]ck.Finding, error) {
	var findings []ck.Finding
	allErrs, _, err := s.ValidateAll(ctx)
	if err != nil {
		return nil, err
	}

	for id, errs := range allErrs {
		for _, e := range errs {
			sev := ck.SeverityWarning
			if e.Field == "signature" || e.Field == "format" {
				sev = ck.SeverityError
			}
			msg := e.Message
			if e.Field == "signature" {
				msg = fmt.Sprintf("🚨 Direct Mutation Detected. Run `docket fix %s` to repair.", id)
			}
			findings = append(findings, ck.Finding{
				TicketID: id,
				Rule:     "V001",
				Message:  msg,
				Severity: sev,
			})
		}
	}
	return findings, nil
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
	cfg, err := ticket.LoadConfig(s.RepoRoot)
	if err != nil {
		cfg = ticket.DefaultConfig()
	}
	var states []ticket.State
	for _, st := range cfg.OpenStates() {
		states = append(states, ticket.State(st))
	}
	return s.ListTickets(ctx, store.Filter{States: states})
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "apply low-risk automatic fixes")
	checkCmd.Flags().BoolVar(&checkDoctor, "doctor", false, "run comprehensive system health checks")
	rootCmd.AddCommand(checkCmd)
}
