package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldThenApplyTemplatesWithMinimalEdits(t *testing.T) {
	h := newFakeRepoHarness(t)

	ticketScaffoldOut, err := h.run("ticket", "scaffold")
	if err != nil {
		t.Fatalf("ticket scaffold failed: %v\n%s", err, ticketScaffoldOut)
	}
	var ticketSpec map[string]any
	if err := json.Unmarshal([]byte(ticketScaffoldOut), &ticketSpec); err != nil {
		t.Fatalf("parse ticket scaffold failed: %v\n%s", err, ticketScaffoldOut)
	}
	ticketBody := ticketSpec["ticket"].(map[string]any)
	ticketBody["title"] = "Scaffold ticket from integration test"
	ticketBody["description"] = "Minimal edit from scaffold for apply integration validation."
	ticketSpecPath := h.writeJSONSpec("scaffold/ticket-spec.json", ticketSpec)

	ticketApplyOut, err := h.run("--automation", "--format", "json", "ticket", "apply", "--spec", ticketSpecPath)
	if err != nil {
		t.Fatalf("ticket apply from scaffold failed: %v\n%s", err, ticketApplyOut)
	}
	if !strings.Contains(ticketApplyOut, "\"action\": \"created\"") {
		t.Fatalf("expected ticket apply create result, got: %s", ticketApplyOut)
	}

	backlogScaffoldOut, err := h.run("backlog", "scaffold")
	if err != nil {
		t.Fatalf("backlog scaffold failed: %v\n%s", err, backlogScaffoldOut)
	}
	var backlogSpec map[string]any
	if err := json.Unmarshal([]byte(backlogScaffoldOut), &backlogSpec); err != nil {
		t.Fatalf("parse backlog scaffold failed: %v\n%s", err, backlogScaffoldOut)
	}
	tickets := backlogSpec["tickets"].([]any)
	entry0 := tickets[0].(map[string]any)
	entry0["title"] = "Scaffold epic"
	entry0["description"] = "Minimal backlog scaffold edit for apply integration."
	entry0["parent"] = "TKT-001"
	backlogSpecPath := h.writeJSONSpec("scaffold/backlog-spec.json", backlogSpec)

	backlogApplyOut, err := h.run("--automation", "--format", "json", "backlog", "apply", "--spec", backlogSpecPath)
	if err != nil {
		t.Fatalf("backlog apply from scaffold failed: %v\n%s", err, backlogApplyOut)
	}
	if !strings.Contains(backlogApplyOut, "\"created_ids\"") {
		t.Fatalf("expected created_ids in backlog apply output, got: %s", backlogApplyOut)
	}

	ticketScaffoldFixture := h.writeFixture(filepath.Join("scaffold", "ticket-scaffold.json"), []byte(ticketScaffoldOut))
	backlogScaffoldFixture := h.writeFixture(filepath.Join("scaffold", "backlog-scaffold.json"), []byte(backlogScaffoldOut))
	ticketApplyFixture := h.writeFixture(filepath.Join("scaffold", "ticket-apply-result.json"), []byte(ticketApplyOut))
	backlogApplyFixture := h.writeFixture(filepath.Join("scaffold", "backlog-apply-result.json"), []byte(backlogApplyOut))
	if _, err := os.Stat(ticketScaffoldFixture); err != nil {
		t.Fatalf("expected scaffold fixture on disk: %v", err)
	}
	t.Logf("scaffold fixtures: %s | %s | %s | %s", ticketScaffoldFixture, backlogScaffoldFixture, ticketApplyFixture, backlogApplyFixture)
}
