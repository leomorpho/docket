package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestWrapUpReportsActionableNextStepsWhenNotReady(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-969", 969, ticket.State("todo"), []ticket.AcceptanceCriterion{
		{Description: "finish implementation", Done: false},
	})

	out, err := h.run("wrap-up", "TKT-969")
	if err != nil {
		t.Fatalf("wrap-up human failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Wrap-up for TKT-969: NOT READY") {
		t.Fatalf("expected NOT READY status, got:\n%s", out)
	}
	if !strings.Contains(out, "Next steps:") {
		t.Fatalf("expected actionable next steps in output, got:\n%s", out)
	}
	if !strings.Contains(out, "docket ac list TKT-969") || !strings.Contains(out, "docket update TKT-969 --handoff -") {
		t.Fatalf("expected AC and handoff remediation steps, got:\n%s", out)
	}

	jsonOut, err := h.run("wrap-up", "TKT-969", "--format", "json")
	if err != nil {
		t.Fatalf("wrap-up json failed: %v\n%s", err, jsonOut)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal wrap-up json failed: %v\n%s", err, jsonOut)
	}
	if payload["ready"] != false {
		t.Fatalf("expected ready=false, got %#v", payload["ready"])
	}
	nextSteps, ok := payload["next_steps"].([]any)
	if !ok || len(nextSteps) == 0 {
		t.Fatalf("expected next_steps in json output, got %#v", payload["next_steps"])
	}
}

func TestWrapUpReportsReadyTicketAndReviewTransitionHint(t *testing.T) {
	h := newFakeRepoHarness(t)
	cfg := ticket.DefaultConfig()
	sections := append([]string{}, cfg.HandoffSections...)
	handoff := strings.Join(sections, "\n\n")
	now := time.Now().UTC().Truncate(time.Second)
	if err := local.New(h.repo).CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-970",
		Seq:         970,
		Title:       "Ready ticket",
		Description: "ready wrap-up ticket",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Handoff:     handoff,
		AC: []ticket.AcceptanceCriterion{
			{Description: "done", Done: true, Evidence: "verified"},
		},
	}); err != nil {
		t.Fatalf("seed ready ticket failed: %v", err)
	}

	out, err := h.run("wrap-up", "TKT-970")
	if err != nil {
		t.Fatalf("wrap-up human failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Wrap-up for TKT-970: READY") {
		t.Fatalf("expected READY status, got:\n%s", out)
	}
	if !strings.Contains(out, "docket update TKT-970 --state in-review") {
		t.Fatalf("expected in-review transition hint, got:\n%s", out)
	}

	jsonOut, err := h.run("wrap-up", "TKT-970", "--format", "json")
	if err != nil {
		t.Fatalf("wrap-up json failed: %v\n%s", err, jsonOut)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal wrap-up json failed: %v\n%s", err, jsonOut)
	}
	if payload["ready"] != true {
		t.Fatalf("expected ready=true, got %#v", payload["ready"])
	}
}
