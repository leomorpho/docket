package local

import (
	"context"
	"testing"
)

func TestUpsertAndQueryAnnotations(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	initial := []Annotation{
		{TicketID: "TKT-001", FilePath: "a.go", LineNum: 10, Context: "// [TKT-001] A"},
		{TicketID: "TKT-001", FilePath: "b.go", LineNum: 20, Context: "// [TKT-001] B"},
		{TicketID: "TKT-002", FilePath: "a.go", LineNum: 30, Context: "// [TKT-002] C"},
	}
	if err := s.UpsertAnnotations(ctx, initial); err != nil {
		t.Fatalf("UpsertAnnotations(initial) failed: %v", err)
	}

	byTicket, err := s.GetAnnotationsByTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetAnnotationsByTicket failed: %v", err)
	}
	if len(byTicket) != 2 {
		t.Fatalf("expected 2 annotations for TKT-001, got %d", len(byTicket))
	}

	byFile, err := s.GetAnnotationsByFile(ctx, "a.go")
	if err != nil {
		t.Fatalf("GetAnnotationsByFile failed: %v", err)
	}
	if len(byFile) != 2 {
		t.Fatalf("expected 2 annotations for a.go, got %d", len(byFile))
	}

	// Upsert semantics: replace prior rows, don't append duplicates.
	replacement := []Annotation{
		{TicketID: "TKT-001", FilePath: "a.go", LineNum: 11, Context: "// [TKT-001] updated"},
	}
	if err := s.UpsertAnnotations(ctx, replacement); err != nil {
		t.Fatalf("UpsertAnnotations(replacement) failed: %v", err)
	}

	byTicket, err = s.GetAnnotationsByTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetAnnotationsByTicket failed: %v", err)
	}
	if len(byTicket) != 1 || byTicket[0].LineNum != 11 {
		t.Fatalf("expected one replaced row at line 11, got %+v", byTicket)
	}
}
