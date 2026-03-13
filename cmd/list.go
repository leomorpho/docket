package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	listState           string
	listLabels          []string
	listMaxPriority     int
	listOnlyUnblocked   bool
	listIncludeArchived bool
)

type listRow struct {
	t     *ticket.Ticket
	depth int
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List tickets",
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
		rows := buildListRows(ctx, s, tickets)

		switch format {
		case "json":
			printJSON(cmd, tickets)
		case "context":
			printContext(cmd, rows)
		default:
			printTable(cmd, rows)
		}

		return nil
	},
}

func printTable(cmd *cobra.Command, rows []listRow) {
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No tickets found.")
		return
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATE\tPRI\tTITLE\tLABELS")
	for _, row := range rows {
		t := row.t
		pStr := fmt.Sprintf("P%d", t.Priority)
		lStr := strings.Join(t.Labels, ",")
		id := t.ID
		if row.depth > 0 {
			id = strings.Repeat("  ", row.depth) + "↳ " + id
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, t.State, pStr, t.Title, lStr)
	}
	w.Flush()
}

func printContext(cmd *cobra.Command, rows []listRow) {
	if len(rows) == 0 {
		return
	}

	for _, row := range rows {
		t := row.t
		blockedStr := ""
		if t.IsBlocked() {
			blockedStr = " | BLOCKED by " + strings.Join(t.BlockedBy, ",")
		} else {
			if len(t.Labels) > 0 {
				blockedStr = " | labels:" + strings.Join(t.Labels, ",")
			}
		}
		title := t.Title
		if row.depth > 0 {
			title = strings.Repeat("  ", row.depth) + "↳ " + title
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] P%d %-11s | %-28s%s\n", t.ID, t.Priority, t.State, title, blockedStr)
	}
}

func buildListRows(ctx context.Context, s *local.Store, tickets []*ticket.Ticket) []listRow {
	if len(tickets) == 0 {
		return nil
	}
	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		out := make([]listRow, 0, len(tickets))
		for _, t := range tickets {
			out = append(out, listRow{t: t, depth: 0})
		}
		return out
	}

	inSet := make(map[string]*ticket.Ticket, len(tickets))
	for _, t := range tickets {
		inSet[t.ID] = t
	}

	var roots []*ticket.Ticket
	for _, t := range tickets {
		if t.Parent == "" {
			roots = append(roots, t)
			continue
		}
		if _, ok := inSet[t.Parent]; !ok {
			roots = append(roots, t)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].Priority != roots[j].Priority {
			return roots[i].Priority < roots[j].Priority
		}
		return roots[i].CreatedAt.Before(roots[j].CreatedAt)
	})

	var out []listRow
	seen := make(map[string]bool, len(tickets))
	var walk func(t *ticket.Ticket, depth int)
	walk = func(t *ticket.Ticket, depth int) {
		if seen[t.ID] {
			return
		}
		seen[t.ID] = true
		out = append(out, listRow{t: t, depth: depth})
		for _, child := range idx.Children[t.ID] {
			if c, ok := inSet[child.ID]; ok {
				walk(c, depth+1)
			}
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	for _, t := range tickets {
		if !seen[t.ID] {
			out = append(out, listRow{t: t, depth: 0})
		}
	}
	return out
}

func init() {
	listCmd.Flags().StringVar(&listState, "state", "open", "filter by state ('open' = all open states from config, or a specific state name)")
	listCmd.Flags().StringSliceVar(&listLabels, "label", []string{}, "filter by label (repeatable)")
	listCmd.Flags().IntVar(&listMaxPriority, "priority", 0, "max priority to show")
	listCmd.Flags().BoolVar(&listOnlyUnblocked, "unblocked", false, "exclude blocked tickets")
	listCmd.Flags().BoolVar(&listIncludeArchived, "include-archived", false, "include archived tickets")

	rootCmd.AddCommand(listCmd)
}
