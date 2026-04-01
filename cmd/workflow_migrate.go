package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var workflowMigrateApply bool

type workflowMigrationTicketChange struct {
	TicketID        string   `json:"ticket_id"`
	FromState       string   `json:"from_state,omitempty"`
	ToState         string   `json:"to_state,omitempty"`
	RemovedBlockers []string `json:"removed_blockers,omitempty"`
}

type workflowMigrationReport struct {
	ConfigMigrated bool                            `json:"config_migrated"`
	Applied        bool                            `json:"applied"`
	Changes        []workflowMigrationTicketChange `json:"changes,omitempty"`
}

var workflowMigrateCmd = &cobra.Command{
	Use:   "workflow-migrate",
	Short: "Migrate the shipped legacy workflow to the north-star state model",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		if !supportsNorthStarWorkflowMigration(cfg) {
			return fmt.Errorf("workflow migration only supports the shipped legacy/default workflow; migrate custom workflows manually")
		}

		report, err := planWorkflowMigration(context.Background(), repo, cfg)
		if err != nil {
			return err
		}
		report.Applied = workflowMigrateApply
		if workflowMigrateApply {
			if err := applyWorkflowMigration(context.Background(), repo, report); err != nil {
				return err
			}
		}

		if format == "json" {
			printJSON(cmd, report)
			return nil
		}
		renderWorkflowMigrationHuman(cmd, report)
		return nil
	},
}

func planWorkflowMigration(ctx context.Context, repoRoot string, cfg *ticket.Config) (workflowMigrationReport, error) {
	s := local.New(repoRoot)
	all, err := s.ListTickets(ctx, store.Filter{IncludeArchived: true})
	if err != nil {
		return workflowMigrationReport{}, err
	}
	fullTickets := make([]*ticket.Ticket, 0, len(all))
	byID := make(map[string]*ticket.Ticket, len(all))
	children := make(map[string][]*ticket.Ticket)
	for _, listed := range all {
		full, err := s.GetTicket(ctx, listed.ID)
		if err != nil {
			return workflowMigrationReport{}, err
		}
		if full == nil {
			continue
		}
		fullTickets = append(fullTickets, full)
		byID[full.ID] = full
	}
	for _, full := range fullTickets {
		if strings.TrimSpace(full.Parent) == "" {
			continue
		}
		children[full.Parent] = append(children[full.Parent], full)
	}

	mappedStates := make(map[string]string, len(fullTickets))
	for _, t := range fullTickets {
		mappedStates[t.ID] = northStarStateForMigration(string(t.State))
	}

	newCfg := ticket.DefaultConfig()
	changes := make([]workflowMigrationTicketChange, 0)
	for _, t := range fullTickets {
		change := workflowMigrationTicketChange{TicketID: t.ID}
		if mapped := mappedStates[t.ID]; mapped != string(t.State) {
			change.FromState = string(t.State)
			change.ToState = mapped
		}
		for _, blockerID := range t.BlockedBy {
			blocker := byID[blockerID]
			if blocker == nil {
				change.RemovedBlockers = append(change.RemovedBlockers, blockerID)
				continue
			}
			if len(children[blockerID]) > 0 {
				change.RemovedBlockers = append(change.RemovedBlockers, blockerID)
				continue
			}
			if !newCfg.BlocksDependents(ticket.State(mappedStates[blockerID])) {
				change.RemovedBlockers = append(change.RemovedBlockers, blockerID)
			}
		}
		if change.FromState != "" || len(change.RemovedBlockers) > 0 {
			sort.Strings(change.RemovedBlockers)
			changes = append(changes, change)
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].TicketID < changes[j].TicketID })
	return workflowMigrationReport{
		ConfigMigrated: !usesNorthStarStates(cfg),
		Changes:        changes,
	}, nil
}

func applyWorkflowMigration(ctx context.Context, repoRoot string, report workflowMigrationReport) error {
	if report.ConfigMigrated {
		if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
			return err
		}
	}
	if len(report.Changes) == 0 {
		return nil
	}
	s := local.New(repoRoot)
	for _, change := range report.Changes {
		t, err := s.GetTicket(ctx, change.TicketID)
		if err != nil {
			return err
		}
		if t == nil {
			continue
		}
		if change.ToState != "" {
			t.State = ticket.State(change.ToState)
		}
		if len(change.RemovedBlockers) > 0 {
			keep := make([]string, 0, len(t.BlockedBy))
			for _, blockerID := range t.BlockedBy {
				if containsWorkflowMigrationString(change.RemovedBlockers, blockerID) {
					continue
				}
				keep = append(keep, blockerID)
			}
			t.BlockedBy = keep
		}
		if err := s.UpdateTicket(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func renderWorkflowMigrationHuman(cmd *cobra.Command, report workflowMigrationReport) {
	mode := "dry-run"
	if report.Applied {
		mode = "applied"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Workflow migration %s.\n", mode)
	if report.ConfigMigrated {
		fmt.Fprintln(cmd.OutOrStdout(), "Config: legacy workflow will be replaced with the north-star default.")
	}
	if len(report.Changes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No ticket changes required.")
		return
	}
	for _, change := range report.Changes {
		parts := []string{change.TicketID}
		if change.FromState != "" {
			parts = append(parts, fmt.Sprintf("state %s -> %s", change.FromState, change.ToState))
		}
		if len(change.RemovedBlockers) > 0 {
			parts = append(parts, fmt.Sprintf("remove blockers [%s]", strings.Join(change.RemovedBlockers, ", ")))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", strings.Join(parts, "; "))
	}
	if !report.Applied {
		fmt.Fprintln(cmd.OutOrStdout(), "Apply with: docket workflow-migrate --apply")
	}
}

func supportsNorthStarWorkflowMigration(cfg *ticket.Config) bool {
	if cfg == nil {
		return false
	}
	allowed := map[string]bool{
		"backlog": true, "todo": true, "in-progress": true, "in-review": true, "done": true, "archived": true,
		"draft": true, "ready": true, "running": true, "validated": true,
	}
	for name := range cfg.States {
		if !allowed[name] {
			return false
		}
	}
	return true
}

func usesNorthStarStates(cfg *ticket.Config) bool {
	if cfg == nil {
		return false
	}
	for _, state := range []string{"draft", "ready", "running", "validated", "archived"} {
		if _, ok := cfg.States[state]; !ok {
			return false
		}
	}
	return true
}

func northStarStateForMigration(state string) string {
	switch strings.TrimSpace(state) {
	case "backlog":
		return "draft"
	case "todo":
		return "ready"
	case "in-progress":
		return "running"
	case "in-review", "done":
		return "validated"
	default:
		return strings.TrimSpace(state)
	}
}

func containsWorkflowMigrationString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func init() {
	workflowMigrateCmd.Flags().BoolVar(&workflowMigrateApply, "apply", false, "write the migrated config and ticket updates")
	rootCmd.AddCommand(workflowMigrateCmd)
}
