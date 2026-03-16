package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
    {"ref": "child-a", "title": "Child A", "description": "First child", "parent_ref": "epic", "blocked_by": ["epic"]},
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
	if len(childB.BlockedBy) != 1 || childB.BlockedBy[0] != "TKT-002" {
		t.Fatalf("expected child B blocked_by TKT-002, got %#v", childB.BlockedBy)
	}
}

func TestBacklogApplyRollbackOnPartialFailure(t *testing.T) {
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
    {"ref": "child", "title": "Child", "description": "Child with invalid parent ID", "parent": "TKT-999"}
  ]
}`
	specPath := writeSpecFile(t, tmpDir, "rollback.json", spec)

	_, _, err := runRootCommand(t, "backlog", "apply", "--spec", specPath)
	if err == nil {
		t.Fatal("expected backlog apply to fail")
	}

	cfg, err := ticket.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("load config after failure: %v", err)
	}
	if cfg.Counter != 0 {
		t.Fatalf("expected counter rollback to 0, got %d", cfg.Counter)
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
