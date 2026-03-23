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

func TestWrapUpUsesConfiguredWorkflowRoleStates(t *testing.T) {
	h := newFakeRepoHarness(t)
	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"coding"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"coding": {
				Label:            "Coding",
				Open:             true,
				Column:           1,
				Next:             []string{"testing"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"testing": {
				Label:            "Testing",
				Open:             true,
				Column:           2,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           3,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   4,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState:    "queued",
		DefaultPriority: 10,
		HandoffSections: []string{"Current state", "Decisions made"},
	}
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := local.New(h.repo).CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-971",
		Seq:         971,
		Title:       "Custom wrap-up",
		Description: "custom wrap-up ticket",
		State:       "testing",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Handoff:     "Current state\n\nDecisions made",
		AC: []ticket.AcceptanceCriterion{
			{Description: "done", Done: true, Evidence: "verified"},
		},
	}); err != nil {
		t.Fatalf("seed custom wrap-up ticket failed: %v", err)
	}

	out, err := h.run("wrap-up", "TKT-971")
	if err != nil {
		t.Fatalf("wrap-up human failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "docket update TKT-971 --state qa") {
		t.Fatalf("expected configured review transition hint, got:\n%s", out)
	}
}

func TestWrapUpSuggestsIntermediaryActiveStateBeforeReviewWhenRequired(t *testing.T) {
	h := newFakeRepoHarness(t)

	cfg := ticket.DefaultConfig()
	cfg.States = map[string]ticket.StateConfig{
		"queued": {
			Label:            "Queued",
			Open:             true,
			Column:           0,
			Next:             []string{"coding"},
			Roles:            []string{"intake"},
			Startable:        true,
			BlocksDependents: true,
		},
		"coding": {
			Label:            "Coding",
			Open:             true,
			Column:           1,
			Next:             []string{"testing"},
			Roles:            []string{"active"},
			BlocksDependents: true,
		},
		"testing": {
			Label:            "Testing",
			Open:             true,
			Column:           2,
			Next:             []string{"qa"},
			Roles:            []string{"active"},
			BlocksDependents: true,
		},
		"qa": {
			Label:            "QA",
			Open:             true,
			Column:           3,
			Next:             []string{"shipped"},
			Roles:            []string{"review"},
			Reviewable:       true,
			BlocksDependents: true,
		},
		"shipped": {
			Label:    "Shipped",
			Open:     false,
			Column:   4,
			Next:     []string{},
			Roles:    []string{"completed"},
			Terminal: true,
		},
	}
	cfg.DefaultState = "queued"
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("save custom workflow config failed: %v", err)
	}

	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-972",
		Seq:         972,
		Title:       "Intermediary wrap-up",
		Description: "Wrap-up should suggest next intermediary active state.",
		State:       "coding",
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		AC: []ticket.AcceptanceCriterion{
			{Description: "all good", Done: true},
		},
		Handoff: "**Current state:** coding\n\n**Decisions made:** done\n\n**Files touched:** cmd/wrap_up.go\n\n**Remaining work:** move to testing before review\n\n**AC status:** complete",
	}); err != nil {
		t.Fatalf("seed intermediary wrap-up ticket failed: %v", err)
	}

	out, err := h.run("wrap-up", "TKT-972")
	if err != nil {
		t.Fatalf("wrap-up human failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "docket update TKT-972 --state testing") {
		t.Fatalf("expected intermediary active transition hint, got:\n%s", out)
	}
}
