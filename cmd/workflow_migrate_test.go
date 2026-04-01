package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestWorkflowMigrateDryRunSupportsCurrentRepoShapeWithoutMutatingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	oldApply := workflowMigrateApply
	repo = tmpDir
	format = "human"
	workflowMigrateApply = false
	t.Cleanup(func() {
		repo = oldRepo
		format = oldFormat
		workflowMigrateApply = oldApply
	})

	if err := ticket.SaveConfig(tmpDir, currentWorkflowMigrationConfig()); err != nil {
		t.Fatalf("save current-style config failed: %v", err)
	}
	seedCurrentWorkflowMigrationFixture(t, tmpDir)

	configBefore := readWorkflowMigrationFixtureFile(t, filepath.Join(tmpDir, ".docket", "config.json"))
	manifestBefore := readWorkflowMigrationFixtureFile(t, filepath.Join(tmpDir, ".docket", "manifest.json"))
	todoBefore := readWorkflowMigrationFixtureFile(t, filepath.Join(tmpDir, ".docket", "tickets", "TKT-401.md"))

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"workflow-migrate", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow-migrate current-style dry-run failed: %v\n%s", err, out.String())
	}

	configAfter := readWorkflowMigrationFixtureFile(t, filepath.Join(tmpDir, ".docket", "config.json"))
	manifestAfter := readWorkflowMigrationFixtureFile(t, filepath.Join(tmpDir, ".docket", "manifest.json"))
	todoAfter := readWorkflowMigrationFixtureFile(t, filepath.Join(tmpDir, ".docket", "tickets", "TKT-401.md"))

	if !bytes.Equal(configAfter, configBefore) {
		t.Fatalf("expected dry-run to leave config untouched")
	}
	if !bytes.Equal(manifestAfter, manifestBefore) {
		t.Fatalf("expected dry-run to leave manifest untouched")
	}
	if !bytes.Equal(todoAfter, todoBefore) {
		t.Fatalf("expected dry-run to leave ticket files untouched")
	}
	if !strings.Contains(out.String(), "TKT-401; state todo -> ready; remove blockers [TKT-400]") {
		t.Fatalf("expected dry-run output to describe todo mapping and coordination blocker removal, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Apply with: docket workflow-migrate --apply") {
		t.Fatalf("expected dry-run apply hint, got:\n%s", out.String())
	}
}

func TestWorkflowMigrateApplyPreservesCustomStatesAndRewritesCurrentManifestData(t *testing.T) {
	tmpDir := t.TempDir()
	oldRepo := repo
	oldFormat := format
	oldApply := workflowMigrateApply
	repo = tmpDir
	format = "human"
	workflowMigrateApply = false
	t.Cleanup(func() {
		repo = oldRepo
		format = oldFormat
		workflowMigrateApply = oldApply
	})

	if err := ticket.SaveConfig(tmpDir, currentWorkflowMigrationConfig()); err != nil {
		t.Fatalf("save current-style config failed: %v", err)
	}
	seedCurrentWorkflowMigrationFixture(t, tmpDir)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"workflow-migrate", "--apply"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("workflow-migrate current-style apply failed: %v\n%s", err, out.String())
	}

	cfg, err := ticket.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	stale, ok := cfg.States["stale"]
	if !ok {
		t.Fatalf("expected migrated config to retain custom stale state")
	}
	if stale.Label != "Stale" || stale.Column != 5 || stale.Open {
		t.Fatalf("unexpected stale state after migration: %#v", stale)
	}
	if !containsWorkflowMigrationString(stale.Next, "draft") || !containsWorkflowMigrationString(stale.Next, "archived") {
		t.Fatalf("expected stale state transitions preserved, got %#v", stale.Next)
	}

	store := local.New(tmpDir)
	assertWorkflowMigrationTicketState(t, store, "TKT-400", "draft")
	assertWorkflowMigrationTicketState(t, store, "TKT-401", "ready")
	assertWorkflowMigrationTicketState(t, store, "TKT-402", "validated")
	assertWorkflowMigrationTicketState(t, store, "TKT-403", "ready")
	assertWorkflowMigrationTicketState(t, store, "TKT-404", "stale")

	coordBlocked, err := store.GetTicket(context.Background(), "TKT-401")
	if err != nil {
		t.Fatalf("GetTicket(TKT-401) error = %v", err)
	}
	if len(coordBlocked.BlockedBy) != 0 {
		t.Fatalf("expected coordination blocker pruned, got %#v", coordBlocked.BlockedBy)
	}

	doneBlocked, err := store.GetTicket(context.Background(), "TKT-403")
	if err != nil {
		t.Fatalf("GetTicket(TKT-403) error = %v", err)
	}
	if len(doneBlocked.BlockedBy) != 0 {
		t.Fatalf("expected completed blocker pruned, got %#v", doneBlocked.BlockedBy)
	}

	manifest := readWorkflowMigrationManifest(t, tmpDir)
	assertWorkflowMigrationManifestState(t, manifest, "TKT-400", "draft")
	assertWorkflowMigrationManifestState(t, manifest, "TKT-401", "ready")
	assertWorkflowMigrationManifestState(t, manifest, "TKT-402", "validated")
	assertWorkflowMigrationManifestState(t, manifest, "TKT-403", "ready")
	assertWorkflowMigrationManifestState(t, manifest, "TKT-404", "stale")

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

func currentWorkflowMigrationConfig() *ticket.Config {
	cfg := ticket.DefaultConfig()
	cfg.States["stale"] = ticket.StateConfig{
		Label:            "Stale",
		Open:             false,
		Column:           5,
		Next:             []string{"draft", "archived"},
		Terminal:         false,
		Startable:        false,
		Reviewable:       false,
		BlocksDependents: false,
	}
	return cfg
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

func seedCurrentWorkflowMigrationFixture(t *testing.T, repoRoot string) {
	t.Helper()
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	tickets := []*ticket.Ticket{
		{
			ID:          "TKT-400",
			Seq:         400,
			Title:       "Current-style epic",
			State:       "draft",
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Current repo-shaped epic that should stay draft while its legacy children migrate.",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-401",
			Seq:         401,
			Title:       "Legacy todo child",
			State:       "todo",
			Priority:    1,
			Parent:      "TKT-400",
			BlockedBy:   []string{"TKT-400"},
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy todo leaf blocked by a coordination parent so migration must prune the blocker.",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-402",
			Seq:         402,
			Title:       "Legacy done blocker",
			State:       "done",
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy completed blocker that should migrate to validated and stop blocking dependents.",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac", Done: true, Evidence: "ok"}},
		},
		{
			ID:          "TKT-403",
			Seq:         403,
			Title:       "Legacy todo dependent",
			State:       "todo",
			Priority:    1,
			BlockedBy:   []string{"TKT-402"},
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Legacy todo leaf blocked by a completed ticket so migration must drop the stale blocker.",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-404",
			Seq:         404,
			Title:       "Custom stale leaf",
			State:       "stale",
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Custom stale state from the current repo shape should be preserved through migration.",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
	}
	for _, tkt := range tickets {
		if err := store.CreateTicket(context.Background(), tkt); err != nil {
			t.Fatalf("CreateTicket(%s) failed: %v", tkt.ID, err)
		}
	}

	manifest := readWorkflowMigrationManifest(t, repoRoot)
	manifest.Warning = "DO NOT EDIT .docket/tickets/*.md OR .docket/manifest.json DIRECTLY. Use `docket` commands only."
	writeWorkflowMigrationManifest(t, repoRoot, manifest)
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

func assertWorkflowMigrationManifestState(t *testing.T, manifest local.Manifest, id, want string) {
	t.Helper()
	entry, ok := manifest.Tickets[id]
	if !ok {
		t.Fatalf("expected manifest entry for %s, got %#v", id, manifest.Tickets)
	}
	if entry.State != want {
		t.Fatalf("manifest state for %s = %q, want %q", id, entry.State, want)
	}
}

func readWorkflowMigrationFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture file %s: %v", path, err)
	}
	return data
}

func readWorkflowMigrationManifest(t *testing.T, repoRoot string) local.Manifest {
	t.Helper()
	data := readWorkflowMigrationFixtureFile(t, filepath.Join(repoRoot, ".docket", "manifest.json"))
	var manifest local.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return manifest
}

func writeWorkflowMigrationManifest(t *testing.T, repoRoot string, manifest local.Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(repoRoot, ".docket", "manifest.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
