package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestLifecycleEventsSimulatedRunOrderAndSchemaFixtures(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-940", 940, ticket.State("todo"), []ticket.AcceptanceCriterion{
		{Description: "start flow AC"},
	})
	h.seedTicket("TKT-941", 941, ticket.State("todo"), []ticket.AcceptanceCriterion{
		{Description: "failing runnable AC", Run: "false"},
	})

	if out, err := h.run("bootstrap", "--adapter", "codex"); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}
	if out, err := h.run("start", "--format", "json"); err != nil {
		t.Fatalf("start failed: %v\n%s", err, out)
	}

	h.t.Setenv("DOCKET_HOOK_AC_ENFORCE", "1")
	hookOut, err := h.run("__hook-ac-check", "TKT-941")
	if err == nil {
		t.Fatalf("expected hook failure to emit tool.failure event, output=%s", hookOut)
	}

	events, err := lifecycle.Load(h.repo)
	if err != nil {
		t.Fatalf("loading lifecycle log failed: %v", err)
	}
	if len(events) != 7 {
		t.Fatalf("expected 7 lifecycle events (start + failing hook run), got %d", len(events))
	}

	gotTypes := make([]string, 0, len(events))
	schemaResult := make([]map[string]any, 0, len(events))
	for i, ev := range events {
		report := lifecycle.ValidateEvent(ev)
		if !report.Valid() {
			t.Fatalf("event %d (%s) failed schema validation: %#v", i, ev.Type, report.Errors)
		}
		gotTypes = append(gotTypes, ev.Type)
		schemaResult = append(schemaResult, map[string]any{
			"index":   i,
			"type":    ev.Type,
			"valid":   report.Valid(),
			"version": report.SchemaVersion,
		})
	}

	wantOrder := []string{
		lifecycle.EventRunStart,
		lifecycle.EventPhaseEnd,
		lifecycle.EventRunEnd,
		lifecycle.EventRunStart,
		lifecycle.EventToolFailure,
		lifecycle.EventPhaseEnd,
		lifecycle.EventRunEnd,
	}
	for i, want := range wantOrder {
		if gotTypes[i] != want {
			t.Fatalf("unexpected event order at %d: want %q got %q (%v)", i, want, gotTypes[i], gotTypes)
		}
	}

	logData, err := os.ReadFile(lifecycle.LogPath(h.repo))
	if err != nil {
		t.Fatalf("read lifecycle log fixture failed: %v", err)
	}
	schemaData, err := json.MarshalIndent(schemaResult, "", "  ")
	if err != nil {
		t.Fatalf("marshal schema result failed: %v", err)
	}
	orderSummary := []byte(fmt.Sprintf("event_order=%v\n", gotTypes))

	logFixture := h.writeFixture(filepath.Join("lifecycle", "events.jsonl"), logData)
	schemaFixture := h.writeFixture(filepath.Join("lifecycle", "schema-validation.json"), append(schemaData, '\n'))
	orderFixture := h.writeFixture(filepath.Join("lifecycle", "event-order.txt"), orderSummary)
	t.Logf("lifecycle fixtures: %s | %s | %s", logFixture, schemaFixture, orderFixture)
}
