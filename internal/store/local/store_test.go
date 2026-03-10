package local

import (
	"context"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestStoreRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	t1 := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Test Ticket",
		State:       ticket.StateTodo,
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "This is a test description.\nIt has multiple lines.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "AC 1", Done: true, Evidence: "it works"},
			{Description: "AC 2", Done: false},
		},
		Plan: []ticket.PlanStep{
			{Description: "Step 1", Status: "done", Notes: "done it"},
			{Description: "Step 2", Status: "pending"},
		},
		Comments: []ticket.Comment{
			{At: now, Author: "human:tester", Body: "First comment"},
		},
		Handoff: "Final handoff",
	}

	// 1. Create
	if err := s.CreateTicket(ctx, t1); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	// 2. Get and compare
	got, err := s.GetTicket(ctx, t1.ID)
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}

	if got.Title != t1.Title {
		t.Errorf("Title mismatch: %q != %q", got.Title, t1.Title)
	}
	if got.Description != t1.Description {
		t.Errorf("Description mismatch: %q != %q", got.Description, t1.Description)
	}
	if len(got.AC) != len(t1.AC) {
		t.Fatalf("AC length mismatch: %d != %d", len(got.AC), len(t1.AC))
	}
	if got.AC[0].Description != t1.AC[0].Description || got.AC[0].Done != t1.AC[0].Done || got.AC[0].Evidence != t1.AC[0].Evidence {
		t.Errorf("AC[0] mismatch: %+v != %+v", got.AC[0], t1.AC[0])
	}
	if len(got.Plan) != len(t1.Plan) {
		t.Fatalf("Plan length mismatch: %d != %d", len(got.Plan), len(t1.Plan))
	}
	if got.Plan[0].Description != t1.Plan[0].Description || got.Plan[0].Status != t1.Plan[0].Status || got.Plan[0].Notes != t1.Plan[0].Notes {
		t.Errorf("Plan[0] mismatch: %+v != %+v", got.Plan[0], t1.Plan[0])
	}
	if len(got.Comments) != len(t1.Comments) {
		t.Fatalf("Comments length mismatch: %d != %d", len(got.Comments), len(t1.Comments))
	}
	if got.Comments[0].Body != t1.Comments[0].Body || got.Comments[0].Author != t1.Comments[0].Author {
		t.Errorf("Comments[0] mismatch: %+v != %+v", got.Comments[0], t1.Comments[0])
	}

	// 3. List
	list, err := s.ListTickets(ctx, store.Filter{})
	if err != nil {
		t.Fatalf("ListTickets failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 ticket in list, got %d", len(list))
	}
}

func TestStoreUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	t1 := &ticket.Ticket{
		ID:    "TKT-001",
		Title: "Initial Title",
		State: ticket.StateTodo,
	}
	s.CreateTicket(ctx, t1)

	// Add comment
	c := ticket.Comment{At: time.Now().UTC().Truncate(time.Second), Author: "human:tester", Body: "New comment"}
	if err := s.AddComment(ctx, t1.ID, c); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}

	// Update fields
	t1.Title = "Updated Title"
	t1.State = ticket.StateInProgress
	if err := s.UpdateTicket(ctx, t1); err != nil {
		t.Fatalf("UpdateTicket failed: %v", err)
	}

	// Verify
	got, _ := s.GetTicket(ctx, t1.ID)
	if got.Title != "Updated Title" {
		t.Errorf("Title not updated: %s", got.Title)
	}
	if len(got.Comments) != 1 || got.Comments[0].Body != "New comment" {
		t.Errorf("Comments lost or wrong: %v", got.Comments)
	}
}

func TestStoreFilter(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Title: "T1", State: ticket.StateTodo, Priority: 2})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Title: "T2", State: ticket.StateInProgress, Priority: 1})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Title: "T3", State: ticket.StateArchived, Priority: 1})

	// Default filter (no archived)
	list, _ := s.ListTickets(ctx, store.Filter{})
	if len(list) != 2 {
		t.Errorf("expected 2 non-archived tickets, got %d", len(list))
	}
	if list[0].ID != "TKT-002" {
		t.Errorf("expected TKT-002 first (higher priority), got %s", list[0].ID)
	}

	// State filter
	list, _ = s.ListTickets(ctx, store.Filter{States: []ticket.State{ticket.StateTodo}})
	if len(list) != 1 || list[0].ID != "TKT-001" {
		t.Errorf("State filter failed: %v", list)
	}
}

func TestActivityBumpsUpdatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	now := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	t1 := &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Activity test",
		State:       ticket.StateTodo,
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:tester",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}
	if err := s.CreateTicket(ctx, t1); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	if err := s.AddComment(ctx, "TKT-001", ticket.Comment{At: time.Now().UTC(), Author: "human", Body: "note"}); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	afterComment, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if !afterComment.UpdatedAt.After(now) {
		t.Fatalf("expected updated_at bump after comment: %s <= %s", afterComment.UpdatedAt, now)
	}

	beforeCommit := afterComment.UpdatedAt
	time.Sleep(1100 * time.Millisecond)
	if err := s.LinkCommit(ctx, "TKT-001", "abc123"); err != nil {
		t.Fatalf("LinkCommit failed: %v", err)
	}
	afterCommit, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if !afterCommit.UpdatedAt.After(beforeCommit) {
		t.Fatalf("expected updated_at bump after link commit: %s <= %s", afterCommit.UpdatedAt, beforeCommit)
	}
}
