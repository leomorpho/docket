package local

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestParseDescriptionKeepsNestedHeadings(t *testing.T) {
	raw := `---
id: TKT-238
seq: 238
state: in-progress
priority: 1
created_at: "2026-03-16T00:00:00Z"
updated_at: "2026-03-16T00:00:00Z"
created_by: human:test
write_hash: ignored
---

# TKT-238: Parser hardening

## Description
Context paragraph.

## Nested Heading Inside Description
This heading should remain in Description content.

### Third-level heading
Still description text.

## Acceptance Criteria
- [ ] parser test coverage
`

	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !strings.Contains(got.Description, "## Nested Heading Inside Description") {
		t.Fatalf("expected nested H2 to be preserved in description, got:\n%s", got.Description)
	}
	if !strings.Contains(got.Description, "### Third-level heading") {
		t.Fatalf("expected nested H3 to be preserved in description, got:\n%s", got.Description)
	}
	if len(got.AC) != 1 || got.AC[0].Description != "parser test coverage" {
		t.Fatalf("expected AC to parse normally, got %#v", got.AC)
	}
}

func TestNestedHeadingRoundtripAndValidate(t *testing.T) {
	tmp := t.TempDir()
	if err := ticket.SaveConfig(tmp, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := New(tmp)
	now := time.Now().UTC().Truncate(time.Second)
	desc := "Context line.\n\n## Nested Heading\nNested heading content that should stay in description.\n\n### Deep Heading\nMore details."
	tkt := &ticket.Ticket{
		ID:          "TKT-500",
		Seq:         500,
		State:       ticket.State("backlog"),
		Priority:    1,
		Title:       "Nested heading ticket",
		Description: desc,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		AC:          []ticket.AcceptanceCriterion{{Description: "validate parser"}},
	}
	if err := s.CreateTicket(context.Background(), tkt); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	got, err := s.GetTicket(context.Background(), "TKT-500")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if got.Description != desc {
		t.Fatalf("description changed after roundtrip\nwant:\n%s\n\ngot:\n%s", desc, got.Description)
	}

	errs, _, err := s.ValidateFile("TKT-500")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors for nested heading description, got %#v", errs)
	}
}
