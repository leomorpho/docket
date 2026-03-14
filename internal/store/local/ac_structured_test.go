package local

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestParseStructuredAcceptanceCriteriaMetadata(t *testing.T) {
	raw := `---
id: TKT-777
seq: 777
state: todo
priority: 1
created_at: "2026-03-14T00:00:00Z"
updated_at: "2026-03-14T00:00:00Z"
created_by: human:test
write_hash: ignored
---

# TKT-777: Structured AC

## Description
desc

## Acceptance Criteria
- [ ] Human verification gate
  kind: human
  applies_to: cli | user-facing
  verification_steps: Run smoke suite | Validate output manually
  preserves: CLI output schema
`
	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(got.AC) != 1 {
		t.Fatalf("expected 1 AC, got %d", len(got.AC))
	}
	ac := got.AC[0]
	if ac.Kind != "human" {
		t.Fatalf("expected kind=human, got %q", ac.Kind)
	}
	if len(ac.AppliesTo) != 2 || ac.AppliesTo[0] != "cli" || ac.AppliesTo[1] != "user-facing" {
		t.Fatalf("unexpected applies_to: %#v", ac.AppliesTo)
	}
	if len(ac.VerificationSteps) != 2 {
		t.Fatalf("unexpected verification_steps: %#v", ac.VerificationSteps)
	}
	if len(ac.Preserves) != 1 || ac.Preserves[0] != "CLI output schema" {
		t.Fatalf("unexpected preserves: %#v", ac.Preserves)
	}
}

func TestRenderStructuredAcceptanceCriteriaMetadata(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID:          "TKT-778",
		Seq:         778,
		Title:       "Structured Writer",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		AC: []ticket.AcceptanceCriterion{
			{
				Description:       "Verify manually",
				Kind:              "human",
				AppliesTo:         []string{"cli"},
				VerificationSteps: []string{"Run command", "Review output"},
				Preserves:         []string{"Exit codes"},
			},
		},
	}
	rendered, err := render(tkt)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	for _, expected := range []string{
		"kind: human",
		"applies_to: cli",
		"verification_steps: Run command | Review output",
		"preserves: Exit codes",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered output missing %q:\n%s", expected, rendered)
		}
	}
}

func TestValidateStructuredAcceptanceCriteriaRules(t *testing.T) {
	tmp := t.TempDir()
	s := New(tmp)
	if err := ticket.SaveConfig(tmp, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{
		ID:          "TKT-779",
		Seq:         779,
		Title:       "Structured Validation",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "This description has enough words to avoid quality warnings interfering with this test case.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "Human gate", Kind: "human"},
		},
	}
	if err := s.CreateTicket(context.Background(), tkt); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}
	errs, _, err := s.ValidateFile("TKT-779")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	found := false
	for _, e := range errs {
		if e.Field == "ac[0].verification_steps" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected verification_steps validation error, got %#v", errs)
	}
}
