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
	listFull            bool
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

		useWorkableView := !listFull
		if listState != "" && listState != "open" {
			if !cfg.IsValidState(listState) {
				return fmt.Errorf("invalid state: %s", listState)
			}
			f.States = []ticket.State{ticket.State(listState)}
			useWorkableView = false
		} else {
			// Default and "open" both use the config-defined open states.
			for _, s := range cfg.OpenStates() {
				f.States = append(f.States, ticket.State(s))
			}
		}

		var tickets []*ticket.Ticket
		if useWorkableView {
			tickets, err = workableTickets(ctx, s, cfg, f)
			if err != nil {
				return fmt.Errorf("listing workable tickets: %w", err)
			}
		} else {
			tickets, err = s.ListTickets(ctx, f)
			if err != nil {
				return fmt.Errorf("listing tickets: %w", err)
			}
		}
		rows := buildListRows(ctx, s, tickets, listFull)

		switch format {
		case "json":
			printJSON(cmd, tickets)
		case "context":
			printContext(cmd, rows, useWorkableView, cfg)
		default:
			printTable(cmd, rows, useWorkableView, cfg)
		}
		return nil
	},
}

func printTable(cmd *cobra.Command, rows []listRow, workableView bool, cfg *ticket.Config) {
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), emptyListMessage(workableView, cfg))
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

func printContext(cmd *cobra.Command, rows []listRow, workableView bool, cfg *ticket.Config) {
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), emptyListMessage(workableView, cfg))
		return
	}

	s := local.New(repo)
	ctx := context.Background()
	for _, row := range rows {
		t := row.t
		blockedStr := ""
		unresolved, err := s.UnresolvedBlockers(ctx, t)
		if err == nil && len(unresolved) > 0 {
			blockedStr = " | BLOCKED by " + strings.Join(unresolved, ",")
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

func emptyListMessage(workableView bool, cfg *ticket.Config) string {
	if !workableView {
		return "No tickets found."
	}

	startable := startableStatesSummary(cfg)
	if startable == "none configured" {
		return "No workable tickets found. Startable states in current config: none configured."
	}
	return fmt.Sprintf("No workable tickets found. Startable states in current config: %s.", startable)
}

func startableStatesSummary(cfg *ticket.Config) string {
	if cfg == nil {
		return "none configured"
	}
	states := cfg.StartableStates()
	if len(states) == 0 {
		return "none configured"
	}
	return strings.Join(states, ", ")
}

func buildListRows(ctx context.Context, s *local.Store, tickets []*ticket.Ticket, full bool) []listRow {
	if len(tickets) == 0 {
		return nil
	}
	if !full {
		out := make([]listRow, 0, len(tickets))
		for _, t := range tickets {
			out = append(out, listRow{t: t, depth: 0})
		}
		return out
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
	listCmd.Flags().BoolVar(&listFull, "full", false, "show the full matching ticket graph instead of only workable tickets")

	rootCmd.AddCommand(listCmd)
}
