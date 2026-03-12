package cmd

import (
	"context"
	"fmt"
	"sort"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var statusParallel bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show docket status and parallel work safety",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !statusParallel {
			fmt.Fprintln(cmd.OutOrStdout(), "Use `docket status --parallel` for in-progress ticket matrix.")
			return nil
		}

		s := local.New(repo)
		tickets, err := s.ListTickets(context.Background(), store.Filter{
			States:          []ticket.State{ticket.State("in-progress")},
			IncludeArchived: true,
		})
		if err != nil {
			return err
		}
		relations, _ := loadRelations(repo)
		lockState, _ := refreshLockClaims(repo)
		lockByID := map[string]map[string]bool{}
		for _, l := range lockState.Locks {
			files := map[string]bool{}
			for _, f := range l.Files {
				files[f] = true
			}
			lockByID[l.TicketID] = files
		}

		ids := make([]string, 0, len(tickets))
		for _, t := range tickets {
			ids = append(ids, t.ID)
		}
		sort.Strings(ids)
		fmt.Fprintln(cmd.OutOrStdout(), "Parallel matrix (safe/risky):")
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				a, b := ids[i], ids[j]
				reason := parallelReason(a, b, relations.Relations, lockByID)
				if reason == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  safe:  %s <-> %s\n", a, b)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  risky: %s <-> %s (%s)\n", a, b, reason)
				}
			}
		}
		return nil
	},
}

func parallelReason(a, b string, relations []relationEntry, lockByID map[string]map[string]bool) string {
	for _, r := range relations {
		if r.Relation == "parallel-safe" && ((r.From == a && r.To == b) || (r.From == b && r.To == a)) {
			return ""
		}
		if r.Relation == "blocks" && ((r.From == a && r.To == b) || (r.From == b && r.To == a)) {
			return "relation blocks"
		}
		if r.Relation == "depends-on" && ((r.From == a && r.To == b) || (r.From == b && r.To == a)) {
			return "relation depends-on"
		}
	}
	for f := range lockByID[a] {
		if lockByID[b][f] {
			return "file overlap"
		}
	}
	return ""
}

func init() {
	statusCmd.Flags().BoolVar(&statusParallel, "parallel", false, "show parallel safety matrix")
	rootCmd.AddCommand(statusCmd)
}
