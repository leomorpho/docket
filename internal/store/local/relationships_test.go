package local

import (
	"context"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestRelationshipIndex(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	
	// Create hierarchy: T1 -> T2 -> T3, T1 -> T4
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "T2", State: "todo", Priority: 1, Parent: "TKT-001", CreatedAt: now, UpdatedAt: now, CreatedBy: "me"})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Seq: 3, Title: "T3", State: "todo", Priority: 1, Parent: "TKT-002", CreatedAt: now, UpdatedAt: now, CreatedBy: "me"})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-004", Seq: 4, Title: "T4", State: "todo", Priority: 1, Parent: "TKT-001", CreatedAt: now, UpdatedAt: now, CreatedBy: "me"})

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex failed: %v", err)
	}

	// 1. Build index
	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		t.Fatalf("BuildRelationshipIndex failed: %v", err)
	}
	
	if len(idx.ByID) != 4 {
		t.Errorf("expected 4 tickets in ByID, got %d", len(idx.ByID))
	}
	if idx.ByID["TKT-001"].Title != "T1" {
		t.Errorf("expected TKT-001 title T1, got %s", idx.ByID["TKT-001"].Title)
	}

	// 2. Test Descendants
	desc1 := idx.Descendants("TKT-001")
	if len(desc1) != 3 {
		t.Errorf("expected 3 descendants for TKT-001, got %d", len(desc1))
	}
	
	desc3 := idx.Descendants("TKT-003")
	if len(desc3) != 0 {
		t.Errorf("expected 0 descendants for TKT-003, got %d", len(desc3))
	}

	descNone := idx.Descendants("TKT-999")
	if len(descNone) != 0 {
		t.Errorf("expected 0 descendants for TKT-999, got %d", len(descNone))
	}
	// Verify order (depth-first usually)
	// T1's children are T2 and T4. T2's child is T3.
	// Order could be [T2, T3, T4] or [T4, T2, T3] depending on sort.
	// Since both T2 and T4 have same priority and created_at, it's deterministic based on ID sort?
	// BuildRelationshipIndex sorts by Priority then CreatedAt.
	
	// 3. Test ComputeDepth
	d1 := idx.ComputeDepth("TKT-001")
	if d1 != 0 {
		t.Errorf("expected depth 0 for root TKT-001, got %d", d1)
	}
	d3 := idx.ComputeDepth("TKT-003")
	if d3 != 2 {
		t.Errorf("expected depth 2 for TKT-003, got %d", d3)
	}

	// 4. Test ParentDepth (uses cache)
	pd3, _ := s.ParentDepth(ctx, "TKT-003")
	if pd3 != 2 {
		t.Errorf("expected parent depth 2 for TKT-003, got %d", pd3)
	}
}

func TestValidateParentRef(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"})

	// 1. Valid parent
	t2 := &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "T2", State: "todo", Priority: 1, Parent: "TKT-001", CreatedAt: now, UpdatedAt: now, CreatedBy: "me"}
	if err := s.validateParentRef(ctx, t2); err != nil {
		t.Errorf("expected TKT-001 to be valid parent: %v", err)
	}

	// 2. Self as parent
	t1 := &ticket.Ticket{ID: "TKT-001", Parent: "TKT-001"}
	if err := s.validateParentRef(ctx, t1); err == nil {
		t.Error("expected error for self-parenting")
	}

	// 3. Non-existent parent
	t3 := &ticket.Ticket{ID: "TKT-003", Parent: "TKT-999"}
	if err := s.validateParentRef(ctx, t3); err == nil {
		t.Error("expected error for non-existent parent")
	}

	// 4. Cycle detection (indirect)
	s.CreateTicket(ctx, t2)
	t1.Parent = "TKT-002"
	if err := s.validateParentRef(ctx, t1); err == nil {
		t.Error("expected error for cycle TKT-001 -> TKT-002 -> TKT-001")
	}
}
