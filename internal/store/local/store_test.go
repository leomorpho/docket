package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
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
		State:       ticket.State("todo"),
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
		State: ticket.State("todo"),
	}
	s.CreateTicket(ctx, t1)

	// Add comment
	c := ticket.Comment{At: time.Now().UTC().Truncate(time.Second), Author: "human:tester", Body: "New comment"}
	if err := s.AddComment(ctx, t1.ID, c); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}

	// Update fields
	t1.Title = "Updated Title"
	t1.State = ticket.State("in-progress")
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

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Title: "T1", State: ticket.State("todo"), Priority: 2})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Title: "T2", State: ticket.State("in-progress"), Priority: 1})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Title: "T3", State: ticket.State("archived"), Priority: 1})

	// Default filter (no archived)
	list, _ := s.ListTickets(ctx, store.Filter{})
	if len(list) != 2 {
		t.Errorf("expected 2 non-archived tickets, got %d", len(list))
	}
	if list[0].ID != "TKT-002" {
		t.Errorf("expected TKT-002 first (higher priority), got %s", list[0].ID)
	}

	// State filter
	list, _ = s.ListTickets(ctx, store.Filter{States: []ticket.State{ticket.State("todo")}})
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
		State:       ticket.State("todo"),
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

func TestMatches(t *testing.T) {
	tkt := &ticket.Ticket{
		ID:       "TKT-001",
		State:    "todo",
		Labels:   []string{"bug", "ui"},
		Priority: 2,
	}

	s := &Store{} // Store used for calling matches (receiver ignored currently)

	tests := []struct {
		f        store.Filter
		expected bool
	}{
		{store.Filter{}, true},
		{store.Filter{States: []ticket.State{"todo"}}, true},
		{store.Filter{States: []ticket.State{"done"}}, false},
		{store.Filter{Labels: []string{"bug"}}, true},
		{store.Filter{Labels: []string{"feature"}}, false},
		{store.Filter{MaxPriority: 3}, true},
		{store.Filter{MaxPriority: 1}, false},
	}

	for i, tt := range tests {
		if got := s.matches(tkt, tt.f); got != tt.expected {
			t.Errorf("test %d failed: matches=%v, want %v", i, got, tt.expected)
		}
	}
}

func TestNextID(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	// Must have config to use NextID
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	id1, seq1, err := s.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != "TKT-001" || seq1 != 1 {
		t.Errorf("expected TKT-001/1, got %s/%d", id1, seq1)
	}

	id2, seq2, _ := s.NextID(ctx)
	if id2 != "TKT-002" || seq2 != 2 {
		t.Errorf("expected TKT-002/2, got %s/%d", id2, seq2)
	}
}

func TestGetRawMissing(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	raw, err := s.GetRaw(context.Background(), "TKT-MISSING")
	if err != nil {
		t.Fatalf("expected no error for missing ticket, got %v", err)
	}
	if raw != "" {
		t.Errorf("expected empty string for missing ticket, got %q", raw)
	}
}

func TestSyncIndex(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// Create a ticket file manually
	ticketDir := filepath.Join(tmpDir, ".docket", "tickets")
	os.MkdirAll(ticketDir, 0755)
	tkt := &ticket.Ticket{
		ID: "TKT-001", Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me",
		Labels: []string{"L1"}, BlockedBy: []string{"TKT-002"}, LinkedCommits: []string{"abc"},
	}
	signTicket(tkt)
	content, _ := render(tkt)
	os.WriteFile(filepath.Join(ticketDir, "TKT-001.md"), []byte(content), 0644)

	// Sync index
	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	// Verify it's in the index
	res, _ := s.ListTickets(ctx, store.Filter{})
	if len(res) != 1 {
		t.Fatal("ticket not found in index after sync")
	}
}

func TestStoreFilterUnblocked(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Title: "T1", State: "todo"})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Title: "T2", State: "todo", BlockedBy: []string{"TKT-001"}})

	res, _ := s.ListTickets(ctx, store.Filter{OnlyUnblocked: true})
	if len(res) != 1 || res[0].ID != "TKT-001" {
		t.Errorf("expected only TKT-001 (unblocked), got %v", res)
	}
}

func TestGetTicketCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	
	ticketDir := filepath.Join(tmpDir, ".docket", "tickets")
	os.MkdirAll(ticketDir, 0755)
	// Invalid frontmatter
	os.WriteFile(filepath.Join(ticketDir, "TKT-001.md"), []byte("---\ninvalid\n---\n# Title"), 0644)
	
	_, err := s.GetTicket(context.Background(), "TKT-001")
	if err == nil {
		t.Error("expected error for corrupt ticket")
	}
}

func TestLinkCommitMissing(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	err := s.LinkCommit(context.Background(), "TKT-MISSING", "sha")
	if err == nil {
		t.Error("expected error linking commit to missing ticket")
	}
}

func TestAddCommentMissing(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	err := s.AddComment(context.Background(), "TKT-MISSING", ticket.Comment{})
	if err == nil {
		t.Error("expected error adding comment to missing ticket")
	}
}

func TestCreateTicketAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	tkt := &ticket.Ticket{ID: "TKT-001", Title: "T1"}
	s.CreateTicket(ctx, tkt)
	err := s.CreateTicket(ctx, tkt)
	if err == nil {
		t.Error("expected error creating already existing ticket")
	}
}

func TestUpdateTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	tkt := &ticket.Ticket{ID: "TKT-001", Title: "T1", State: "todo", CreatedAt: now, UpdatedAt: now, CreatedBy: "me"}
	s.CreateTicket(ctx, tkt)

	// 1. Transition to in-progress
	tkt.State = "in-progress"
	s.UpdateTicket(ctx, tkt)
	res, _ := s.GetTicket(ctx, "TKT-001")
	if res.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}

	// 2. Transition to done
	tkt.State = "done"
	s.UpdateTicket(ctx, tkt)
	res, _ = s.GetTicket(ctx, "TKT-001")
	if res.CompletedAt.IsZero() {
		t.Error("expected CompletedAt to be set")
	}
}
