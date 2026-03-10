package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/ticket"
)

func TestSessionAttachListResolveAndCompressMark(t *testing.T) {
	repo := t.TempDir()
	s := New(repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T", State: ticket.StateTodo, Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	src := filepath.Join(repo, "session.jsonl")
	if err := os.WriteFile(src, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rel, err := s.AttachSession(ctx, "TKT-001", src)
	if err != nil {
		t.Fatalf("AttachSession failed: %v", err)
	}
	if rel == "" {
		t.Fatal("expected relative session path")
	}

	files, err := s.ListSessions(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("sessions count = %d, want 1", len(files))
	}

	path, err := s.ResolveSessionPath(ctx, "TKT-001", "")
	if err != nil {
		t.Fatalf("ResolveSessionPath failed: %v", err)
	}

	compressed, err := s.MarkSessionCompressed(path)
	if err != nil {
		t.Fatalf("MarkSessionCompressed failed: %v", err)
	}
	if filepath.Ext(compressed) != ".compressed" {
		t.Fatalf("expected .compressed suffix, got %s", compressed)
	}
}
