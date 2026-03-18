package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestSessionAttachListResolveAndCompressMark(t *testing.T) {
	repo := t.TempDir()
	s := New(repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T", State: ticket.State("todo"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"}); err != nil {
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
	if got, want := files[0].Path, filepath.Join(repo, ".docket", "local", "tickets", "TKT-001", "sessions", filepath.Base(files[0].Path)); got != want {
		t.Fatalf("session path = %q, want %q", got, want)
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

func TestListSessionsFallsBackToLegacySessionDir(t *testing.T) {
	repo := t.TempDir()
	s := New(repo)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T", State: ticket.State("todo"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me"}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	legacyDir := artifacts.LegacyRepoPath(repo, artifacts.RepoTicketSessions, "TKT-001", "sessions")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "2026-03-18T160000Z.log")
	if err := os.WriteFile(legacyFile, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	files, err := s.ListSessions(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(files) != 1 || files[0].Path != legacyFile {
		t.Fatalf("unexpected legacy sessions: %+v", files)
	}
}
