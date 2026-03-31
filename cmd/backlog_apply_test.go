package cmd

import (
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

func TestBacklogApplyCreatesStableIDMapping(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "tickets": [
    {"ref": "epic", "title": "Epic", "description": "Root epic", "labels": ["feature"]},
    {"ref": "child-a", "title": "Child A", "description": "First child", "parent_ref": "epic"},
    {"ref": "child-b", "title": "Child B", "description": "Second child", "parent_ref": "epic", "blocked_by": ["child-a"]}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "backlog.json", spec)

	out, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath)
	if err != nil {
		t.Fatalf("backlog apply failed: %v", err)
	}

	var res struct {
		CreatedIDs map[string]string `json:"created_ids"`
		Created    []string          `json:"created_order"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse backlog apply output: %v\noutput=%s", err, out)
	}
	if res.CreatedIDs["epic"] != "TKT-001" || res.CreatedIDs["child-a"] != "TKT-002" || res.CreatedIDs["child-b"] != "TKT-003" {
		t.Fatalf("unexpected created id mapping: %#v", res.CreatedIDs)
	}

	s := local.New(tmpDir)
	childA, err := s.GetTicket(context.Background(), "TKT-002")
	if err != nil {
		t.Fatalf("get child A: %v", err)
	}
	childB, err := s.GetTicket(context.Background(), "TKT-003")
	if err != nil {
		t.Fatalf("get child B: %v", err)
	}
	if childA.Parent != "TKT-001" {
		t.Fatalf("expected child A parent TKT-001, got %q", childA.Parent)
	}
	if len(childA.BlockedBy) != 0 {
		t.Fatalf("expected child A to have no blockers, got %#v", childA.BlockedBy)
	}
	if len(childB.BlockedBy) != 1 || childB.BlockedBy[0] != "TKT-002" {
		t.Fatalf("expected child B blocked_by TKT-002, got %#v", childB.BlockedBy)
	}
}

func TestBacklogApplyRejectsNonLeafExecutionBlocker(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "tickets": [
    {"ref": "epic", "title": "Epic", "description": "Root epic"},
    {"ref": "child-a", "title": "Child A", "description": "First child", "parent_ref": "epic", "blocked_by": ["epic"]}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "non-leaf-blocker.json", spec)

	_, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected backlog apply to reject non-leaf blocker")
	}
	if !strings.Contains(err.Error(), "must be a leaf ticket") {
		t.Fatalf("expected leaf-blocker error, got %v", err)
	}
}

func TestBacklogApplyRollbackOnPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	cfg := ticket.DefaultConfig()
	cfg.Counter = 1
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "tickets": [
    {"ref": "epic", "title": "Epic", "description": "Root epic"},
    {"ref": "child", "title": "Child", "description": "Child with invalid parent ID", "parent": "TKT-999"}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "rollback.json", spec)

	_, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected backlog apply to fail")
	}

	loadedCfg, err := ticket.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("load config after failure: %v", err)
	}
	if loadedCfg.Counter != 1 {
		t.Fatalf("expected counter rollback to 1, got %d", loadedCfg.Counter)
	}

	if _, statErr := os.Stat(filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")); !os.IsNotExist(statErr) {
		t.Fatalf("expected rollback to remove created ticket file, stat err=%v", statErr)
	}
}

func TestBacklogApplyIntegrationRelationGraph(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "tickets": [
    {"ref": "epic", "title": "Epic", "description": "Root epic"},
    {"ref": "child-a", "title": "Child A", "description": "First child", "parent_ref": "epic"},
    {"ref": "child-b", "title": "Child B", "description": "Second child", "parent_ref": "epic"}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "graph.json", spec)
	if _, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath); err != nil {
		t.Fatalf("backlog apply failed: %v", err)
	}

	s := local.New(tmpDir)
	idx, err := s.BuildRelationshipIndex(context.Background())
	if err != nil {
		t.Fatalf("build relationship index: %v", err)
	}
	desc := idx.Descendants("TKT-001")
	if len(desc) != 2 {
		t.Fatalf("expected 2 descendants under epic, got %d", len(desc))
	}
}

func TestBacklogApplyUsesConfiguredWorkflowStatesWithIntermediaryState(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	cfg := ticket.DefaultConfig()
	cfg.States = map[string]ticket.StateConfig{
		"queued":  {Label: "Queued", Open: true, Column: 0, Next: []string{"coding"}, Roles: []string{"intake"}, Startable: true, BlocksDependents: true},
		"coding":  {Label: "Coding", Open: true, Column: 1, Next: []string{"testing", "queued"}, Roles: []string{"active"}, BlocksDependents: true},
		"testing": {Label: "Testing", Open: true, Column: 2, Next: []string{"qa", "coding"}, Roles: []string{"active"}, BlocksDependents: true},
		"qa":      {Label: "QA", Open: true, Column: 3, Next: []string{"shipped", "testing"}, Roles: []string{"review"}, Reviewable: true, BlocksDependents: true},
		"shipped": {Label: "Shipped", Open: false, Column: 4, Next: []string{}, Roles: []string{"completed"}, Terminal: true},
	}
	cfg.Workflow = ticket.WorkflowConfig{Version: 1, States: map[string]ticket.WorkflowStateConfig{
		"queued":  {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"intake"}, Open: true, Startable: true, BlocksDependents: true, Next: []string{"coding"}}, Presentation: ticket.WorkflowStatePresentation{Label: "Queued", Column: 0}},
		"coding":  {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"active"}, Open: true, BlocksDependents: true, Next: []string{"testing", "queued"}}, Presentation: ticket.WorkflowStatePresentation{Label: "Coding", Column: 1}},
		"testing": {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"active"}, Open: true, BlocksDependents: true, Next: []string{"qa", "coding"}}, Presentation: ticket.WorkflowStatePresentation{Label: "Testing", Column: 2}},
		"qa":      {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"review"}, Open: true, Reviewable: true, BlocksDependents: true, Next: []string{"shipped", "testing"}}, Presentation: ticket.WorkflowStatePresentation{Label: "QA", Column: 3}},
		"shipped": {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"completed"}, Terminal: true, Next: []string{}}, Presentation: ticket.WorkflowStatePresentation{Label: "Shipped", Column: 4}},
	}}
	cfg.DefaultState = "queued"
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "tickets": [
    {"ref": "epic", "title": "Epic", "description": "Root epic", "state": "coding"},
    {"ref": "child", "title": "Child", "description": "Child item", "parent_ref": "epic", "state": "testing"}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "custom-backlog.json", spec)

	out, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath)
	if err != nil {
		t.Fatalf("backlog apply failed: %v", err)
	}

	var res struct {
		CreatedIDs map[string]string `json:"created_ids"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse output: %v\noutput=%s", err, out)
	}

	s := local.New(tmpDir)
	epic, err := s.GetTicket(context.Background(), res.CreatedIDs["epic"])
	if err != nil {
		t.Fatalf("get epic: %v", err)
	}
	child, err := s.GetTicket(context.Background(), res.CreatedIDs["child"])
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if epic.State != ticket.State("coding") {
		t.Fatalf("epic state = %q, want coding", epic.State)
	}
	if child.State != ticket.State("testing") {
		t.Fatalf("child state = %q, want testing", child.State)
	}
}

func TestBacklogApplyWorklistCreatesDraftTicketsWithStableTitlesAndParent(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	cfg := ticket.DefaultConfig()
	cfg.Counter = 1
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Roadmap Parent",
		Description: "Parent for imported worklist items",
		State:       ticket.State("backlog"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
	}); err != nil {
		t.Fatalf("create parent ticket: %v", err)
	}

	worklistPath := filepath.Join(tmpDir, "worklist.txt")
	worklist := strings.Join([]string{
		"- Build auth middleware",
		"2. Add rate-limit guardrails",
		"* Document rollout plan",
	}, "\n")
	if err := os.WriteFile(worklistPath, []byte(worklist), 0o644); err != nil {
		t.Fatalf("write worklist: %v", err)
	}

	out, _, err := runRootCommand(t, "backlog", "apply", "--worklist", worklistPath, "--parent", "TKT-001")
	if err != nil {
		t.Fatalf("backlog apply --worklist failed: %v", err)
	}

	var res struct {
		Created []string `json:"created_order"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse output: %v\noutput=%s", err, out)
	}
	if len(res.Created) != 3 {
		t.Fatalf("expected 3 created tickets, got %#v", res)
	}

	tickets := make([]*ticket.Ticket, 0, len(res.Created))
	for _, id := range res.Created {
		tkt, err := s.GetTicket(context.Background(), id)
		if err != nil {
			t.Fatalf("get created ticket %s: %v", id, err)
		}
		tickets = append(tickets, tkt)
	}

	wantTitles := []string{
		"Build auth middleware",
		"Add rate-limit guardrails",
		"Document rollout plan",
	}
	for i, tkt := range tickets {
		if tkt.Title != wantTitles[i] {
			t.Fatalf("ticket %d title = %q, want %q", i, tkt.Title, wantTitles[i])
		}
		if tkt.Parent != "TKT-001" {
			t.Fatalf("ticket %d parent = %q, want TKT-001", i, tkt.Parent)
		}
		if !strings.Contains(strings.ToLower(tkt.Description), "worklist") {
			t.Fatalf("ticket %d should remain draft/groomable, got description %q", i, tkt.Description)
		}
	}
}

func TestBacklogApplyWorklistRejectsEmptyInputAndSpecConflict(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	emptyPath := filepath.Join(tmpDir, "empty-worklist.txt")
	if err := os.WriteFile(emptyPath, []byte("\n  \n"), 0o644); err != nil {
		t.Fatalf("write empty worklist: %v", err)
	}
	if _, _, err := runRootCommand(t, "backlog", "apply", "--worklist", emptyPath); err == nil {
		t.Fatal("expected empty worklist to fail")
	}

	specPath := writeSpecFile(t, tmpDir, "spec.json", `{"version":"docket.apply/v1","tickets":[]}`)
	if _, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath, "--worklist", emptyPath); err == nil {
		t.Fatal("expected --spec and --worklist conflict to fail")
	}
}

func TestBacklogApplyRejectsDisconnectedGraph(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	cfg := ticket.DefaultConfig()
	cfg.Counter = 1
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Existing",
		Description: "Existing anchor ticket",
		State:       ticket.State("backlog"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
	}); err != nil {
		t.Fatalf("seed existing ticket: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "tickets": [
    {"ref": "new", "title": "Disconnected", "description": "Not connected to graph"}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "disconnected-backlog.json", spec)

	_, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected disconnected backlog apply to fail")
	}

	if got, getErr := s.GetTicket(context.Background(), "TKT-002"); getErr != nil {
		t.Fatalf("get TKT-002: %v", getErr)
	} else if got != nil {
		t.Fatalf("expected no disconnected ticket created, got %#v", got)
	}

	loadedCfg, cfgErr := ticket.LoadConfig(tmpDir)
	if cfgErr != nil {
		t.Fatalf("load config after failure: %v", cfgErr)
	}
	if loadedCfg.Counter != 1 {
		t.Fatalf("expected counter rollback to 1, got %d", loadedCfg.Counter)
	}
}
