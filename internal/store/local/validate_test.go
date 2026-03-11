package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/ticket"
)

func TestValidateFile(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	// 1. Valid ticket
	now := time.Now().UTC().Truncate(time.Second)
	t1 := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Valid Ticket",
		State:       ticket.State("todo"),
		Priority:    1,
		Labels:      []string{"bug"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "Has description",
		AC:          []ticket.AcceptanceCriterion{{Description: "Has AC", Done: false}},
		Handoff:     "Has handoff",
	}
	s.CreateTicket(ctx, t1)

	errs, warns, err := s.ValidateFile(t1.ID)
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid ticket, got: %v", errs)
	}
	if len(warns) > 0 {
		t.Errorf("expected no warnings for valid ticket, got: %v", warns)
	}

	// 2. Invalid ticket (wrong state, missing description)
	t2 := &ticket.Ticket{
		ID:        "TKT-002",
		Seq:       2,
		Title:     "Invalid Ticket",
		State:     "blocked", // invalid state
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "human:tester",
		// No description, no AC
	}
	s.CreateTicket(ctx, t2)
	errs, _, _ = s.ValidateFile(t2.ID)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors (state, body desc, body ac), got: %d", len(errs))
	}

	// 3. ID mismatch
	// Manually write a file with mismatching ID in frontmatter
	path3 := filepath.Join(tmpDir, ".docket", "tickets", "TKT-003.md")
	t3 := *t1
	t3.ID = "TKT-999"
	content, _ := render(&t3)
	os.WriteFile(path3, []byte(content), 0644)
	errs, _, _ = s.ValidateFile("TKT-003")
	foundIDMismatch := false
	for _, e := range errs {
		if e.Field == "id" {
			foundIDMismatch = true
		}
	}
	if !foundIDMismatch {
		t.Errorf("expected ID mismatch error, but not found in %v", errs)
	}
}

func TestDetectCycles(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	now := time.Now().UTC()
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", BlockedBy: []string{"TKT-002"}, Title: "T1", State: "todo", CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", BlockedBy: []string{"TKT-003"}, Title: "T2", State: "todo", CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", BlockedBy: []string{"TKT-001"}, Title: "T3", State: "todo", CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})

	err := s.detectCycles()
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("expected cycle detected message, got: %v", err)
	}
}

func TestValidateFile_RequiresStructuredHandoffForReviewAndDone(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	t1 := &ticket.Ticket{
		ID:          "TKT-010",
		Seq:         10,
		Title:       "Needs handoff",
		State:       ticket.State("in-review"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		Handoff:     "partial handoff",
	}
	if err := s.CreateTicket(ctx, t1); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	errs, _, err := s.ValidateFile("TKT-010")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected handoff structure validation errors")
	}

	t1.Handoff = "*Last updated: 2026-03-09T15:00:00Z by agent:test*\n\n**Current state:** done.\n\n**Decisions made:** decision.\n\n**Files touched:** file.\n\n**Remaining work:** none.\n\n**AC status:** complete."
	if err := s.UpdateTicket(ctx, t1); err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}
	errs, _, err = s.ValidateFile("TKT-010")
	if err != nil {
		t.Fatalf("ValidateFile failed: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("expected no handoff structure errors, got %v", errs)
	}
}
