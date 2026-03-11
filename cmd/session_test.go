package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestSessionAttachListCompress(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	runGitSession(t, tmpDir, "init")
	runGitSession(t, tmpDir, "config", "user.email", "test@example.com")
	runGitSession(t, tmpDir, "config", "user.name", "Test User")

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "Sess", State: ticket.State("todo"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{Description: "A"}}}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	cfg := ticket.DefaultConfig()
	cfg.HandoffSections = []string{"Current state", "Risks"}
	if err := ticket.SaveConfig(tmpDir, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	sessionFile := filepath.Join(tmpDir, "log.jsonl")
	if err := os.WriteFile(sessionFile, []byte("hello session\n"), 0644); err != nil {
		t.Fatal(err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"session", "attach", "TKT-001", "--file", sessionFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session attach failed: %v", err)
	}
	if !strings.Contains(b.String(), "Session attached to TKT-001") {
		t.Fatalf("unexpected attach output:\n%s", b.String())
	}

	b.Reset()
	rootCmd.SetArgs([]string{"session", "list", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session list failed: %v", err)
	}
	if !strings.Contains(b.String(), "Sessions for TKT-001") {
		t.Fatalf("unexpected list output:\n%s", b.String())
	}

	b.Reset()
	rootCmd.SetArgs([]string{"session", "compress", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session compress prompt failed: %v", err)
	}
	if !strings.Contains(b.String(), "Write a handoff summary") {
		t.Fatalf("expected prompt output, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "**Current state:**") || !strings.Contains(b.String(), "**Risks:**") {
		t.Fatalf("expected prompt sections from config, got:\n%s", b.String())
	}
	if strings.Contains(b.String(), "**Decisions made:**") {
		t.Fatalf("did not expect hardcoded section in prompt, got:\n%s", b.String())
	}

	summary := filepath.Join(tmpDir, "summary.md")
	content := "## Handoff\n\n*Last updated: 2026-03-09T15:00:00Z by agent:test*\n\n**Current state:** done\n\n**Decisions made:** x\n\n**Files touched:** y\n\n**Remaining work:** z\n\n**AC status:** 0/1\n"
	if err := os.WriteFile(summary, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"session", "compress", "TKT-001", "--summary-file", summary})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("session compress apply failed: %v", err)
	}
	if !strings.Contains(b.String(), "Handoff updated") {
		t.Fatalf("unexpected compress output:\n%s", b.String())
	}

	updated, err := s.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("get ticket failed: %v", err)
	}
	if !strings.Contains(updated.Handoff, "Current state") {
		t.Fatalf("handoff not updated: %q", updated.Handoff)
	}

	files, err := s.ListSessions(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("list sessions failed: %v", err)
	}
	compressedFound := false
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".compressed") {
			compressedFound = true
		}
	}
	if !compressedFound {
		t.Fatalf("expected compressed session file")
	}
}

func runGitSession(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
