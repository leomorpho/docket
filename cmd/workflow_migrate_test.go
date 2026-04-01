package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestWorkflowMigrateDryRunLeavesRepoUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	t.Cleanup(func() {
		repo = oldRepo
		format = oldFormat
	})

	if err := ticket.SaveConfig(tmpDir, legacyWorkflowMigrationConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	seedWorkflowMigrationFixture(t, tmpDir)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"workflow-migrate"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow-migrate dry-run failed: %v\n%s", err, out.String())
	}

	store := local.New(tmpDir)
	tkt, err := store.GetTicket(context.Background(), "TKT-201")
	if err != nil {
		t.Fatalf("GetTicket() error = %v", err)
	}
	if tkt.State != ticket.State("todo") {
		t.Fatalf("expected dry-run to leave state unchanged, got %s", tkt.State)
	}
	if !strings.Contains(out.String(), "Apply with: docket workflow-migrate --apply") {
		t.Fatalf("expected apply hint, got:\n%s", out.String())
	}
}

func TestWorkflowMigrateApplyMigratesLegacyStatesAndPrunesInvalidBlockers(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	repo = tmpDir
	format = "human"
	t.Cleanup(func() {
		repo = oldRepo
		format = oldFormat
	})

	if err := ticket.SaveConfig(tmpDir, legacyWorkflowMigrationConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	seedWorkflowMigrationFixture(t, tmpDir)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"workflow-migrate", "--apply"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow-migrate apply failed: %v\n%s", err, out.String())
	}

	cfg, err := ticket.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	for _, state := range []string{"draft", "ready", "running", "validated", "archived"} {
		if _, ok := cfg.States[state]; !ok {
			t.Fatalf("expected migrated config state %s", state)
		}
	}

	store := local.New(tmpDir)
	assertWorkflowMigrationTicketState(t, store, "TKT-200", "draft")
	assertWorkflowMigrationTicketState(t, store, "TKT-201", "ready")
	assertWorkflowMigrationTicketState(t, store, "TKT-202", "validated")
	assertWorkflowMigrationTicketState(t, store, "TKT-203", "ready")

	child, err := store.GetTicket(context.Background(), "TKT-201")
	if err != nil {
		t.Fatalf("GetTicket(TKT-201) error = %v", err)
	}
	if len(child.BlockedBy) != 0 {
		t.Fatalf("expected coordination blocker pruned, got %#v", child.BlockedBy)
	}
	dependent, err := store.GetTicket(context.Background(), "TKT-203")
	if err != nil {
		t.Fatalf("GetTicket(TKT-203) error = %v", err)
	}
	if len(dependent.BlockedBy) != 0 {
		t.Fatalf("expected completed blocker pruned, got %#v", dependent.BlockedBy)
	}
	if !strings.Contains(out.String(), "Workflow migration applied.") {
		t.Fatalf("expected apply summary, got:\n%s", out.String())
	}
}

func legacyWorkflowMigrationConfig() *ticket.Config {
	return &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"backlog":     {Label: "Backlog", Open: true, Column: 0, Next: []string{"todo"}, Roles: []string{"intake"}, Startable: true, BlocksDependents: true},
			"todo":        {Label: "To Do", Open: true, Column: 1, Next: []string{"in-progress"}, Roles: []string{"intake"}, Startable: true, BlocksDependents: true},
			"in-progress": {Label: "In Progress", Open: true, Column: 2, Next: []string{"in-review"}, Roles: []string{"active"}, BlocksDependents: true},
			"in-review":   {Label: "In Review", Open: true, Column: 3, Next: []string{"done"}, Roles: []string{"review"}, Reviewable: true, BlocksDependents: false},
			"done":        {Label: "Done", Open: false, Column: 4, Next: []string{"archived"}, Roles: []string{"completed"}, Terminal: true},
			"archived":    {Label: "Archived", Open: false, Column: 5, Next: []string{}, Roles: []string{"archived"}, Terminal: true},
		},
		DefaultState:    "backlog",
		DefaultPriority: 10,
		HandoffSections: []string{"Current state", "Decisions made", "Files touched", "Remaining work", "AC status"},
	}
}

func seedWorkflowMigrationFixture(t *testing.T, repoRoot string) {
	t.Helper()
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	tickets := []*ticket.Ticket{
		{
			ID:          "TKT-200",
			Seq:         200,
			Title:       "Legacy epic",
			State:       "backlog",
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy epic placeholder",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-201",
			Seq:         201,
			Title:       "Legacy child",
			State:       "todo",
			Priority:    1,
			Parent:      "TKT-200",
			BlockedBy:   []string{"TKT-200"},
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy child task",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-202",
			Seq:         202,
			Title:       "Legacy completed blocker",
			State:       "done",
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy completed task",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac", Done: true, Evidence: "ok"}},
		},
		{
			ID:          "TKT-203",
			Seq:         203,
			Title:       "Legacy dependent",
			State:       "todo",
			Priority:    1,
			BlockedBy:   []string{"TKT-202"},
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy dependent task",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
	}
	for _, tkt := range tickets {
		if err := store.CreateTicket(context.Background(), tkt); err != nil {
			t.Fatalf("CreateTicket(%s) failed: %v", tkt.ID, err)
		}
	}
}

func assertWorkflowMigrationTicketState(t *testing.T, store *local.Store, id, want string) {
	t.Helper()
	tkt, err := store.GetTicket(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTicket(%s) error = %v", id, err)
	}
	if tkt == nil || tkt.State != ticket.State(want) {
		t.Fatalf("ticket %s state = %#v, want %s", id, tkt, want)
	}
}
