package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	listState           string
	listLabels          []string
	listMaxPriority     int
	listOnlyUnblocked   bool
	listIncludeArchived bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tickets",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}

		s := local.New(repo)
		ctx := context.Background()

		f := store.Filter{
			Labels:          listLabels,
			MaxPriority:     listMaxPriority,
			OnlyUnblocked:   listOnlyUnblocked,
			IncludeArchived: listIncludeArchived,
		}

		if listState != "" && listState != "open" {
			if !cfg.IsValidState(listState) {
				return fmt.Errorf("invalid state: %s", listState)
			}
			f.States = []ticket.State{ticket.State(listState)}
		} else {
			// Default and "open" both use the config-defined open states.
			for _, s := range cfg.OpenStates() {
				f.States = append(f.States, ticket.State(s))
			}
		}

		tickets, err := s.ListTickets(ctx, f)
		if err != nil {
			return fmt.Errorf("listing tickets: %w", err)
		}

		switch format {
		case "json":
			printJSON(cmd, tickets)
		case "context":
			printContext(cmd, tickets)
		default:
			printTable(cmd, tickets)
		}

		return nil
	},
}

func printTable(cmd *cobra.Command, tickets []*ticket.Ticket) {
	if len(tickets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No tickets found.")
		return
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATE\tPRI\tTITLE\tLABELS")
	for _, t := range tickets {
		pStr := fmt.Sprintf("P%d", t.Priority)
		lStr := strings.Join(t.Labels, ",")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.State, pStr, t.Title, lStr)
	}
	w.Flush()
}

func printContext(cmd *cobra.Command, tickets []*ticket.Ticket) {
	if len(tickets) == 0 {
		return
	}

	for _, t := range tickets {
		blockedStr := ""
		if t.IsBlocked() {
			blockedStr = " | BLOCKED by " + strings.Join(t.BlockedBy, ",")
		} else {
			if len(t.Labels) > 0 {
				blockedStr = " | labels:" + strings.Join(t.Labels, ",")
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] P%d %-11s | %-28s%s\n", t.ID, t.Priority, t.State, t.Title, blockedStr)
	}
}

func init() {
	listCmd.Flags().StringVar(&listState, "state", "open", "filter by state ('open' = all open states from config, or a specific state name)")
	listCmd.Flags().StringSliceVar(&listLabels, "label", []string{}, "filter by label (repeatable)")
	listCmd.Flags().IntVar(&listMaxPriority, "priority", 0, "max priority to show")
	listCmd.Flags().BoolVar(&listOnlyUnblocked, "unblocked", false, "exclude blocked tickets")
	listCmd.Flags().BoolVar(&listIncludeArchived, "include-archived", false, "include archived tickets")

	rootCmd.AddCommand(listCmd)
}
