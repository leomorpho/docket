package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func readyCheckDescription() string {
	return "Likely paths: cmd/ready.go and cmd/ready_check_test.go. Verify commands: go test ./cmd -run TestReadyCheckCommand -count=1. Out of scope: unrelated workflow migration work. This draft ticket includes enough execution context for a deterministic readiness check command contract."
}

func readyCheckAC() []ticket.AcceptanceCriterion {
	return []ticket.AcceptanceCriterion{
		{Description: "Readiness check reports pass", Run: "test -n ready"},
		{Description: "No state mutation occurs", VerificationSteps: []string{"Inspect ticket state after readiness check"}},
	}
}

func TestReadyCheckCommandReportsAllMissingContractFields(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tk := &ticket.Ticket{
		ID:          "TKT-201",
		Seq:         201,
		Title:       "Draft ticket missing ready contract details",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "This draft is too short and omits the sections agents need.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Only one acceptance criterion and no verification"},
		},
	}
	if err := s.CreateTicket(ctx, tk); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{"ready", "TKT-201"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected readiness check to fail for incomplete draft ticket")
	}

	got := out.String()
	for _, want := range []string{
		"TKT-201",
		"ready_contract.description",
		"at least 30 words of execution context",
		"Likely paths",
		"Out of scope",
		"ready_contract.ac",
		"at least 2 acceptance criteria",
		"ready_contract.verification",
		"explicit verification",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected readiness check output to contain %q, got:\n%s", want, got)
		}
	}

	stored, getErr := s.GetTicket(ctx, "TKT-201")
	if getErr != nil {
		t.Fatalf("get ticket: %v", getErr)
	}
	if stored.State != ticket.State("draft") {
		t.Fatalf("expected readiness check to leave ticket in draft, got %s", stored.State)
	}
}

func TestReadyCheckCommandRejectsNonLeafTickets(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-210",
			Seq:         210,
			Title:       "Parent draft ticket",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			Description: readyCheckDescription(),
			AC:          readyCheckAC(),
		},
		{
			ID:          "TKT-211",
			Seq:         211,
			Title:       "Child draft ticket",
			Parent:      "TKT-210",
			State:       ticket.State("draft"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			Description: readyCheckDescription(),
			AC:          readyCheckAC(),
		},
	} {
		if err := s.CreateTicket(ctx, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{"ready", "TKT-210"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected readiness check to reject non-leaf ticket")
	}
	if !strings.Contains(strings.ToLower(out.String()), "leaf ticket") {
		t.Fatalf("expected non-leaf readiness rejection, got:\n%s", out.String())
	}
}

func TestReadyCheckCommandRechecksReadyTicketWithoutMutation(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := local.New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tk := &ticket.Ticket{
		ID:          "TKT-220",
		Seq:         220,
		Title:       "Already ready leaf ticket",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: readyCheckDescription(),
		AC:          readyCheckAC(),
	}
	if err := s.CreateTicket(ctx, tk); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	rootCmd.SetArgs([]string{"ready", "TKT-220"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected ready ticket recheck to pass, got: %v\n%s", err, out.String())
	}
	if !strings.Contains(strings.ToLower(out.String()), "passes ready contract") {
		t.Fatalf("expected successful readiness check output, got:\n%s", out.String())
	}

	stored, getErr := s.GetTicket(ctx, "TKT-220")
	if getErr != nil {
		t.Fatalf("get ticket: %v", getErr)
	}
	if stored.State != ticket.State("ready") {
		t.Fatalf("expected readiness recheck to keep ready state, got %s", stored.State)
	}
}

func TestReadyCheckCommandJSONReportsStructuredIssues(t *testing.T) {
	h := newFakeRepoHarness(t)

	s := local.New(h.repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-230",
		Seq:         230,
		Title:       "Draft missing verification",
		State:       ticket.State("draft"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:harness-agent",
		Description: "Likely paths: cmd/ready.go. This draft omits verification and out of scope guidance so the readiness check should explain both gaps with machine-readable fields for repair.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Only one AC and no verification"},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	out, err := h.run("--format", "json", "ready", "TKT-230")
	if err == nil {
		t.Fatalf("expected json readiness check to fail, output=%s", out)
	}

	jsonOut, extractErr := extractFirstJSONObject(out)
	if extractErr != nil {
		t.Fatalf("extract json failed: %v\n%s", extractErr, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal readiness json failed: %v\n%s", err, out)
	}
	if payload["ticket_id"] != "TKT-230" {
		t.Fatalf("expected ticket_id TKT-230, got %#v", payload["ticket_id"])
	}
	if payload["ready"] != false {
		t.Fatalf("expected ready=false, got %#v", payload["ready"])
	}
	if payload["state"] != "draft" {
		t.Fatalf("expected state draft, got %#v", payload["state"])
	}
	issues, ok := payload["issues"].([]any)
	if !ok || len(issues) == 0 {
		t.Fatalf("expected structured readiness issues, got %#v", payload["issues"])
	}

	fields := map[string]bool{}
	for _, raw := range issues {
		issue, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected issue object, got %#v", raw)
		}
		field, _ := issue["field"].(string)
		fields[field] = true
	}
	for _, want := range []string{"ready_contract.description", "ready_contract.ac", "ready_contract.verification"} {
		if !fields[want] {
			t.Fatalf("expected readiness json issue for %s, got %#v", want, payload["issues"])
		}
	}
}
