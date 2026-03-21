package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var errHealthFindings = errors.New("health findings present")

type healthIssue struct {
	Code   string `json:"code"`
	Ticket string `json:"ticket,omitempty"`
	Detail string `json:"detail"`
}

type healthStats struct {
	TotalTickets int `json:"total_tickets"`
	Components   int `json:"components"`
}

type healthReport struct {
	OK     bool          `json:"ok"`
	Stats  healthStats   `json:"stats"`
	Issues []healthIssue `json:"issues"`
}

func currentComponentCount(ctx context.Context, repoRoot string) (int, error) {
	report, err := buildHealthReport(ctx, repoRoot)
	if err != nil {
		return 0, err
	}
	return report.Stats.Components, nil
}

func enforceCreateConnectivity(ctx context.Context, s *local.Store, t *ticket.Ticket) error {
	existingIDs, err := listTicketIDs(s.RepoRoot)
	if err != nil {
		return fmt.Errorf("listing existing tickets: %w", err)
	}
	if len(existingIDs) == 0 {
		return nil
	}

	existingSet := make(map[string]struct{}, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[id] = struct{}{}
	}
	if t.Parent != "" {
		if _, ok := existingSet[t.Parent]; ok {
			return nil
		}
	}
	for _, blockerID := range t.BlockedBy {
		if _, ok := existingSet[blockerID]; ok {
			return nil
		}
	}
	for _, blocksID := range t.Blocks {
		if _, ok := existingSet[blocksID]; ok {
			return nil
		}
	}
	return fmt.Errorf("new ticket must connect to the existing ticket graph via --parent, --blocked-by, or --blocks")
}

func enforceMutationConnectivity(ctx context.Context, repoRoot string, beforeComponents int) error {
	afterComponents, err := currentComponentCount(ctx, repoRoot)
	if err != nil {
		return err
	}
	if afterComponents > beforeComponents {
		return fmt.Errorf("mutation disconnected the ticket graph (%d -> %d components)", beforeComponents, afterComponents)
	}
	return nil
}

var healthCmd = &cobra.Command{
	Use:          "health",
	Short:        "Validate ticket graph connectivity",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := buildHealthReport(context.Background(), ticketRepoRoot(repo))
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, report)
		} else {
			if report.OK {
				fmt.Fprintf(cmd.OutOrStdout(), "Ticket graph healthy: %d tickets, %d connected component.\n", report.Stats.TotalTickets, report.Stats.Components)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Ticket graph unhealthy: %d tickets, %d connected components.\n", report.Stats.TotalTickets, report.Stats.Components)
				for _, issue := range report.Issues {
					if issue.Ticket != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  - %s %s: %s\n", issue.Code, issue.Ticket, issue.Detail)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", issue.Code, issue.Detail)
					}
				}
			}
		}

		if !report.OK {
			return errHealthFindings
		}
		return nil
	},
}

func buildHealthReport(ctx context.Context, repoRoot string) (healthReport, error) {
	s := local.New(repoRoot)
	tickets, err := s.ListTickets(ctx, store.Filter{IncludeArchived: false})
	if err != nil {
		return healthReport{}, fmt.Errorf("listing tickets: %w", err)
	}
	for i, tk := range tickets {
		full, err := s.GetTicket(ctx, tk.ID)
		if err != nil {
			return healthReport{}, fmt.Errorf("loading ticket %s: %w", tk.ID, err)
		}
		if full != nil {
			tickets[i] = full
		}
	}

	report := healthReport{
		OK: true,
		Stats: healthStats{
			TotalTickets: len(tickets),
		},
		Issues: []healthIssue{},
	}
	if len(tickets) == 0 {
		return report, nil
	}

	byID := make(map[string]*ticket.Ticket, len(tickets))
	adj := make(map[string]map[string]struct{}, len(tickets))
	for _, t := range tickets {
		byID[t.ID] = t
		adj[t.ID] = make(map[string]struct{})
	}

	var issues []healthIssue
	addEdge := func(a, b string) {
		if a == "" || b == "" || a == b {
			return
		}
		if _, ok := adj[a]; !ok {
			return
		}
		if _, ok := adj[b]; !ok {
			return
		}
		adj[a][b] = struct{}{}
		adj[b][a] = struct{}{}
	}

	for _, t := range tickets {
		if t.Parent != "" {
			if _, ok := byID[t.Parent]; ok {
				addEdge(t.ID, t.Parent)
			} else {
				issues = append(issues, healthIssue{
					Code:   "missing_parent",
					Ticket: t.ID,
					Detail: fmt.Sprintf("parent %s does not exist in the active ticket graph", t.Parent),
				})
			}
		}
		for _, blockerID := range t.BlockedBy {
			if _, ok := byID[blockerID]; ok {
				addEdge(t.ID, blockerID)
			} else {
				issues = append(issues, healthIssue{
					Code:   "missing_blocker",
					Ticket: t.ID,
					Detail: fmt.Sprintf("blocked_by target %s does not exist in the active ticket graph", blockerID),
				})
			}
		}
		for _, blocksID := range t.Blocks {
			if _, ok := byID[blocksID]; ok {
				addEdge(t.ID, blocksID)
			} else {
				issues = append(issues, healthIssue{
					Code:   "missing_blocks_target",
					Ticket: t.ID,
					Detail: fmt.Sprintf("blocks target %s does not exist in the active ticket graph", blocksID),
				})
			}
		}
	}

	relations, err := loadRelations(repoRoot)
	if err != nil {
		return healthReport{}, fmt.Errorf("loading relations: %w", err)
	}
	for _, rel := range relations.Relations {
		fromExists := byID[rel.From] != nil
		toExists := byID[rel.To] != nil
		switch {
		case fromExists && toExists:
			addEdge(rel.From, rel.To)
		case !fromExists:
			issues = append(issues, healthIssue{
				Code:   "missing_relation_endpoint",
				Ticket: rel.To,
				Detail: fmt.Sprintf("relation %s references missing ticket %s", rel.Relation, rel.From),
			})
		default:
			issues = append(issues, healthIssue{
				Code:   "missing_relation_endpoint",
				Ticket: rel.From,
				Detail: fmt.Sprintf("relation %s references missing ticket %s", rel.Relation, rel.To),
			})
		}
	}

	components := connectedComponents(adj)
	report.Stats.Components = len(components)
	if len(components) > 1 {
		report.OK = false
		issues = append(issues, healthIssue{
			Code:   "graph_disconnected",
			Detail: fmt.Sprintf("ticket graph split into %d components", len(components)),
		})
		main := largestComponent(components)
		mainSet := make(map[string]struct{}, len(main))
		for _, id := range main {
			mainSet[id] = struct{}{}
		}
		for _, component := range components {
			if sameComponent(component, main) {
				continue
			}
			for _, id := range component {
				issues = append(issues, healthIssue{
					Code:   "disconnected_ticket",
					Ticket: id,
					Detail: fmt.Sprintf("ticket is outside the main connected component (component size %d)", len(component)),
				})
			}
		}
		_ = mainSet
	}

	if cycleErr := s.DetectCycleValidationError(); cycleErr != nil {
		report.OK = false
		issues = append(issues, healthIssue{
			Code:   "cycle_detected",
			Detail: cycleErr.Message,
		})
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		if issues[i].Ticket != issues[j].Ticket {
			return issues[i].Ticket < issues[j].Ticket
		}
		return issues[i].Detail < issues[j].Detail
	})
	report.Issues = issues
	if len(issues) > 0 {
		report.OK = false
	}
	return report, nil
}

func connectedComponents(adj map[string]map[string]struct{}) [][]string {
	ids := make([]string, 0, len(adj))
	for id := range adj {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	visited := make(map[string]bool, len(adj))
	components := make([][]string, 0)
	for _, start := range ids {
		if visited[start] {
			continue
		}
		component := []string{}
		queue := []string{start}
		visited[start] = true
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)

			neighbors := make([]string, 0, len(adj[current]))
			for next := range adj[current] {
				neighbors = append(neighbors, next)
			}
			sort.Strings(neighbors)
			for _, next := range neighbors {
				if visited[next] {
					continue
				}
				visited[next] = true
				queue = append(queue, next)
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}

	sort.Slice(components, func(i, j int) bool {
		if len(components[i]) != len(components[j]) {
			return len(components[i]) > len(components[j])
		}
		return strings.Join(components[i], ",") < strings.Join(components[j], ",")
	})
	return components
}

func largestComponent(components [][]string) []string {
	if len(components) == 0 {
		return nil
	}
	return components[0]
}

func sameComponent(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
