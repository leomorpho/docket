package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestACCompleteWritesCheckpointAndSessionResume(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-501", Seq: 501, Title: "Checkpoint", State: "todo", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-501", "--step", "1", "--evidence", "done"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete failed: %v", err)
	}
	paths, err := listCheckpointPaths(tmp, "TKT-501")
	if err != nil || len(paths) == 0 {
		t.Fatalf("expected checkpoint after ac complete, err=%v paths=%v", err, paths)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "resume", "TKT-501"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session resume failed: %v", err)
	}
	if !strings.Contains(out.String(), "RESUME_CONTEXT") || !strings.Contains(out.String(), "ac=") {
		t.Fatalf("unexpected resume output: %s", out.String())
	}
}

func TestSessionCompressCheckpointAndListIncludesCheckpoints(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-502", Seq: 502, Title: "Compress", State: "todo", Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})
	// attach session file
	sessionSource := filepath.Join(tmp, "session.log")
	_ = os.WriteFile(sessionSource, []byte("hello"), 0o644)
	if _, err := s.AttachSession(context.Background(), "TKT-502", sessionSource); err != nil {
		t.Fatalf("attach session failed: %v", err)
	}
	summaryPath := filepath.Join(tmp, "summary.md")
	_ = os.WriteFile(summaryPath, []byte("## Handoff\n\nSummary text"), 0o644)

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"session", "compress", "TKT-502", "--summary-file", summaryPath, "--checkpoint"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session compress failed: %v", err)
	}

	paths, err := listCheckpointPaths(tmp, "TKT-502")
	if err != nil || len(paths) == 0 {
		t.Fatalf("expected checkpoint after session compress --checkpoint, err=%v paths=%v", err, paths)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"session", "list", "TKT-502"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Checkpoints for TKT-502") {
		t.Fatalf("expected checkpoint listing, got: %s", out.String())
	}
}
