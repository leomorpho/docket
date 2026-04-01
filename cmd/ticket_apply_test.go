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

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestTicketApplyCreateAndUpdateTransactional(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	createSpec := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Create by apply",
    "description": "Create a fully groomed ticket in one transaction.",
    "priority": 1,
    "state": "draft",
    "labels": ["feature", "llm-only"],
    "ac": ["unit checks", "integration checks"]
  }
}`
	createPath := writeSpecFile(t, tmpDir, "create.json", createSpec)

	out, _, err := runRootCommand(t, "ticket", "apply", "--spec", createPath)
	if err != nil {
		t.Fatalf("ticket apply create failed: %v", err)
	}

	var createRes map[string]any
	if err := json.Unmarshal([]byte(out), &createRes); err != nil {
		t.Fatalf("parse create json output: %v\noutput=%s", err, out)
	}
	if createRes["id"] != "TKT-001" {
		t.Fatalf("expected created id TKT-001, got %#v", createRes["id"])
	}
	if createRes["action"] != "created" {
		t.Fatalf("expected action created, got %#v", createRes["action"])
	}

	s := local.New(tmpDir)
	created, err := s.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("get created ticket: %v", err)
	}
	if created == nil {
		t.Fatal("expected created ticket")
	}
	if len(created.AC) != 2 || created.AC[0].Description != "unit checks" || created.AC[1].Description != "integration checks" {
		t.Fatalf("unexpected created AC: %#v", created.AC)
	}

	updateSpec := `{
  "version": "docket.apply/v1",
  "operation": "update",
  "ticket": {
    "id": "TKT-001",
    "title": "Updated by apply",
    "description": "Updated description from apply command.",
    "labels": ["feature"],
    "ac": ["replacement AC"]
  }
}`
	updatePath := writeSpecFile(t, tmpDir, "update.json", updateSpec)

	out, _, err = runRootCommand(t, "ticket", "apply", "--spec", updatePath)
	if err != nil {
		t.Fatalf("ticket apply update failed: %v", err)
	}
	var updateRes map[string]any
	if err := json.Unmarshal([]byte(out), &updateRes); err != nil {
		t.Fatalf("parse update json output: %v\noutput=%s", err, out)
	}
	if updateRes["id"] != "TKT-001" {
		t.Fatalf("expected updated id TKT-001, got %#v", updateRes["id"])
	}
	if updateRes["action"] != "updated" {
		t.Fatalf("expected action updated, got %#v", updateRes["action"])
	}

	updated, err := s.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("get updated ticket: %v", err)
	}
	if updated.Title != "Updated by apply" {
		t.Fatalf("expected updated title, got %q", updated.Title)
	}
	if len(updated.AC) != 1 || updated.AC[0].Description != "replacement AC" {
		t.Fatalf("expected AC replacement, got %#v", updated.AC)
	}
}

func TestTicketApplyCreateRollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	ticketsDir := filepath.Join(tmpDir, ".docket", "tickets")
	if err := os.MkdirAll(ticketsDir, 0o755); err != nil {
		t.Fatalf("mkdir tickets dir: %v", err)
	}
	if err := os.Chmod(ticketsDir, 0o500); err != nil {
		t.Fatalf("chmod tickets dir: %v", err)
	}
	defer os.Chmod(ticketsDir, 0o755)

	spec := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Will fail",
    "description": "Create should fail and rollback counter.",
    "ac": ["single"]
  }
}`
	specPath := writeSpecFile(t, tmpDir, "fail-create.json", spec)

	_, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected ticket apply create to fail")
	}

	cfg, cfgErr := ticket.LoadConfig(tmpDir)
	if cfgErr != nil {
		t.Fatalf("load config after failure: %v", cfgErr)
	}
	if cfg.Counter != 0 {
		t.Fatalf("expected counter rollback to 0, got %d", cfg.Counter)
	}

	s := local.New(tmpDir)
	tkt, getErr := s.GetTicket(context.Background(), "TKT-001")
	if getErr != nil {
		t.Fatalf("get ticket after failed apply: %v", getErr)
	}
	if tkt != nil {
		t.Fatalf("expected no created ticket after rollback, got %#v", tkt)
	}
}

func TestTicketApplyIntegrationMarkdownAndIndex(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Integration apply",
    "description": "Ensure markdown and sqlite index remain consistent after apply.",
    "labels": ["feature"],
    "ac": ["one", "two"]
  }
}`
	specPath := writeSpecFile(t, tmpDir, "integration.json", spec)
	if _, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath); err != nil {
		t.Fatalf("ticket apply failed: %v", err)
	}

	path := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created markdown: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# TKT-001: Integration apply") {
		t.Fatalf("markdown missing title: %s", content)
	}
	if !strings.Contains(content, "- [ ] one") || !strings.Contains(content, "- [ ] two") {
		t.Fatalf("markdown missing AC entries: %s", content)
	}

	s := local.New(tmpDir)
	listed, err := s.ListTickets(context.Background(), store.Filter{})
	if err != nil {
		t.Fatalf("list tickets from index: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "TKT-001" {
		t.Fatalf("unexpected indexed tickets: %#v", listed)
	}
}

func TestTicketApplyUsesConfiguredWorkflowStates(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	cfg := ticket.DefaultConfig()
	cfg.States = map[string]ticket.StateConfig{
		"queued":   {Label: "Queued", Open: true, Column: 0, Next: []string{"building"}, Roles: []string{"intake"}, Startable: true, BlocksDependents: true},
		"building": {Label: "Building", Open: true, Column: 1, Next: []string{"qa", "queued"}, Roles: []string{"active"}, BlocksDependents: true},
		"qa":       {Label: "QA", Open: true, Column: 2, Next: []string{"shipped", "building"}, Roles: []string{"review"}, Reviewable: true, BlocksDependents: true},
		"shipped":  {Label: "Shipped", Open: false, Column: 3, Next: []string{}, Roles: []string{"completed"}, Terminal: true},
	}
	cfg.Workflow = ticket.WorkflowConfig{Version: 1, States: map[string]ticket.WorkflowStateConfig{
		"queued":   {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"intake"}, Open: true, Startable: true, BlocksDependents: true, Next: []string{"building"}}, Presentation: ticket.WorkflowStatePresentation{Label: "Queued", Column: 0}},
		"building": {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"active"}, Open: true, BlocksDependents: true, Next: []string{"qa", "queued"}}, Presentation: ticket.WorkflowStatePresentation{Label: "Building", Column: 1}},
		"qa":       {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"review"}, Open: true, Reviewable: true, BlocksDependents: true, Next: []string{"shipped", "building"}}, Presentation: ticket.WorkflowStatePresentation{Label: "QA", Column: 2}},
		"shipped":  {Semantics: ticket.WorkflowStateSemantics{Roles: []string{"completed"}, Terminal: true, Next: []string{}}, Presentation: ticket.WorkflowStatePresentation{Label: "Shipped", Column: 3}},
	}}
	cfg.DefaultState = "queued"
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

spec := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Custom state create",
    "description": "Likely paths: cmd/ticket_apply.go and internal/store/local/ready_contract.go. Verify commands: go test ./cmd -run TestTicketApplyUsesConfiguredWorkflowStates -count=1. Out of scope: unrelated scheduler behavior. This ticket is intentionally detailed enough to enter the configured active state directly through ticket apply.",
    "state": "building",
    "ac": ["unit validation remains green", "integration path is exercised"]
  }
}`
	specPath := writeSpecFile(t, tmpDir, "custom-state.json", spec)

	out, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath)
	if err != nil {
		t.Fatalf("ticket apply failed: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse output: %v\noutput=%s", err, out)
	}
	if res["id"] != "TKT-001" {
		t.Fatalf("expected TKT-001, got %#v", res["id"])
	}

	s := local.New(tmpDir)
	created, err := s.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("get created ticket: %v", err)
	}
	if created.State != ticket.State("building") {
		t.Fatalf("state = %q, want building", created.State)
	}
}

func TestTicketApplyCreateRejectsDisconnectedGraph(t *testing.T) {
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
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
	}); err != nil {
		t.Fatalf("seed existing ticket: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Disconnected",
    "description": "This ticket is not connected to existing graph."
  }
}`
	specPath := writeSpecFile(t, tmpDir, "disconnected-create.json", spec)

	_, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected disconnected create to fail")
	}

	if got, getErr := s.GetTicket(context.Background(), "TKT-002"); getErr != nil {
		t.Fatalf("get TKT-002: %v", getErr)
	} else if got != nil {
		t.Fatalf("expected no disconnected ticket created, got %#v", got)
	}
}

func TestTicketApplyRejectsNonLeafExecutionBlocker(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-001",
			Seq:         1,
			Title:       "Parent blocker",
			Description: "Parent blocker",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-002",
			Seq:         2,
			Title:       "Child",
			Description: "Child ticket",
			Parent:      "TKT-001",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
	} {
		if err := s.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	spec := `{
  "version": "docket.apply/v1",
  "operation": "create",
  "ticket": {
    "title": "Blocked leaf",
    "description": "Blocked by parent ticket",
    "blocked_by": ["TKT-001"],
    "ac": ["single"]
  }
}`
	specPath := writeSpecFile(t, tmpDir, "non-leaf-blocker-ticket-apply.json", spec)

	_, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected ticket apply to reject non-leaf blocker")
	}
	if !strings.Contains(err.Error(), "must be a leaf ticket") {
		t.Fatalf("expected leaf-blocker error, got %v", err)
	}
}

func TestTicketApplyUpdateRollsBackDisconnectedMutation(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	cfg := ticket.DefaultConfig()
	cfg.Counter = 2
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	root := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Root",
		Description: "Root ticket",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
	}
	child := &ticket.Ticket{
		ID:          "TKT-002",
		Seq:         2,
		Title:       "Child",
		Description: "Child ticket",
		State:       ticket.State("draft"),
		Priority:    1,
		Parent:      "TKT-001",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
	}
	if err := s.CreateTicket(context.Background(), root); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if err := s.CreateTicket(context.Background(), child); err != nil {
		t.Fatalf("seed child: %v", err)
	}

	spec := `{
  "version": "docket.apply/v1",
  "operation": "update",
  "ticket": {
    "id": "TKT-002",
    "parent": ""
  }
}`
	specPath := writeSpecFile(t, tmpDir, "disconnect-update.json", spec)

	_, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected disconnected update to fail")
	}

	updated, getErr := s.GetTicket(context.Background(), "TKT-002")
	if getErr != nil {
		t.Fatalf("reload TKT-002: %v", getErr)
	}
	if updated.Parent != "TKT-001" {
		t.Fatalf("expected rollback to keep parent TKT-001, got %q", updated.Parent)
	}
}

func runRootCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), errOut.String(), err
}

func writeSpecFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write spec file %s: %v", name, err)
	}
	return path
}

func TestTicketApplyStdinSpec(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdinSpec := `{"version":"docket.apply/v1","operation":"create","ticket":{"title":"stdin","description":"stdin apply create","ac":["done"]}}`
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString(stdinSpec); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	_ = w.Close()
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	out, _, err := runRootCommand(t, "ticket", "apply", "--spec", "-")
	if err != nil {
		t.Fatalf("ticket apply via stdin failed: %v", err)
	}
	if !strings.Contains(out, "\"id\": \"TKT-001\"") {
		t.Fatalf("unexpected stdin apply output: %s", out)
	}
}

func TestTicketApplyDeterministicUpdatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	base := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		State:       ticket.State("draft"),
		Priority:    1,
		Title:       "Base",
		Description: "Base ticket for deterministic update timestamp coverage.",
		AC:          []ticket.AcceptanceCriterion{{Description: "old"}},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
	}
	if err := s.CreateTicket(context.Background(), base); err != nil {
		t.Fatalf("create base ticket: %v", err)
	}

	spec := `{"version":"docket.apply/v1","operation":"update","ticket":{"id":"TKT-001","ac":["new"]}}`
	specPath := writeSpecFile(t, tmpDir, "update-ts.json", spec)
	if _, _, err := runRootCommand(t, "ticket", "apply", "--spec", specPath); err != nil {
		t.Fatalf("ticket apply update failed: %v", err)
	}

	updated, err := s.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("get updated ticket: %v", err)
	}
	if updated.UpdatedAt.Before(now) {
		t.Fatalf("expected updated_at to be >= original timestamp: before=%s after=%s", now, updated.UpdatedAt)
	}
}
