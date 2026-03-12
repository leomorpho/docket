package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestACAddCompleteCheckAndList(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-001", Seq: 1, Title: "AC", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc",
		AC: []ticket.AcceptanceCriterion{{Description: "First"}, {Description: "Tests pass"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"ac", "add", "TKT-001", "--desc", "Integration tests pass"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac add failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-001", "--step", "1", "--evidence", "done"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete index failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-001", "--step", "Tests", "--evidence", "go test passed"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete substring failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"ac", "check", "TKT-001"})
	err := rootCmd.Execute()
	if !errors.Is(err, errACIncomplete) {
		t.Fatalf("expected incomplete error, got: %v", err)
	}
	if !strings.Contains(b.String(), "incomplete") {
		t.Fatalf("unexpected check output: %s", b.String())
	}
	if strings.Contains(b.String(), "Usage:") {
		t.Fatalf("did not expect usage output on AC gate failure:\n%s", b.String())
	}

	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"ac", "check", "TKT-001", "--format", "json"})
	err = rootCmd.Execute()
	if !errors.Is(err, errACIncomplete) {
		t.Fatalf("expected incomplete error for json, got: %v", err)
	}
	jsonOut := b.String()
	if idx := strings.Index(jsonOut, "\nUsage:"); idx != -1 {
		jsonOut = jsonOut[:idx]
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("json parse failed: %v\noutput:\n%s", err, b.String())
	}
	if payload["complete"] != false {
		t.Fatalf("complete = %v, want false", payload["complete"])
	}

	// Complete remaining AC and check success.
	format = "human"
	b.Reset()
	rootCmd.SetArgs([]string{"ac", "complete", "TKT-001", "--step", "Integration", "--evidence", "ci green"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac complete remaining failed: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"ac", "check", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac check expected success: %v", err)
	}

	b.Reset()
	rootCmd.SetArgs([]string{"ac", "list", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac list failed: %v", err)
	}
	if !strings.Contains(b.String(), "Acceptance criteria for TKT-001") {
		t.Fatalf("unexpected list output: %s", b.String())
	}
}

func TestACCheck_NoItemsIsComplete(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-002", Seq: 2, Title: "No AC", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"ac", "check", "TKT-002"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected success for no AC, got: %v", err)
	}
}

func TestACRunCommandAndDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-003", Seq: 3, Title: "Runnable AC", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "add", "TKT-003", "--desc", "Create marker file", "--run", "echo ok > .ac-check.tmp"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac add with run failed: %v", err)
	}

	// Dry-run should not execute.
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"ac", "check", "TKT-003", "--dry-run"})
	if err := rootCmd.Execute(); !errors.Is(err, errACIncomplete) {
		t.Fatalf("expected incomplete on dry-run, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".ac-check.tmp")); err == nil {
		t.Fatal("expected dry-run to not execute command")
	}
	if !strings.Contains(b.String(), "Dry-run commands:") {
		t.Fatalf("expected dry-run output, got: %s", b.String())
	}

	// Real run should execute and complete.
	b.Reset()
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"ac", "check", "TKT-003"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected runnable AC to pass, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".ac-check.tmp")); err != nil {
		t.Fatalf("expected command side effect file, got err: %v", err)
	}
}

func TestACUpdateAndRemove(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-004", Seq: 4, Title: "Update AC", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc",
		AC: []ticket.AcceptanceCriterion{
			{Description: "First AC"},
			{Description: "Second AC"},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "update", "TKT-004", "--step", "1", "--desc", "Updated AC", "--run", "echo hi"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac update failed: %v", err)
	}
	updated, _ := s.GetTicket(context.Background(), "TKT-004")
	if updated.AC[0].Description != "Updated AC" || updated.AC[0].Run != "echo hi" {
		t.Fatalf("ac update not persisted: %+v", updated.AC[0])
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"ac", "remove", "TKT-004", "--step", "2", "--yes"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ac remove failed: %v", err)
	}
	updated, _ = s.GetTicket(context.Background(), "TKT-004")
	if len(updated.AC) != 1 {
		t.Fatalf("expected AC length 1 after remove, got %d", len(updated.AC))
	}
}

func TestHookACCheckRespectsSkipEnv(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	t.Setenv("DOCKET_SKIP_AC", "1")

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-005", Seq: 5, Title: "Hook AC", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc",
		AC: []ticket.AcceptanceCriterion{{Description: "Prose AC"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"__hook-ac-check", "TKT-005"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected skip env to bypass hook ac check, got: %v", err)
	}
}

func TestHookACCheckInteractivePrompt(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-006", Seq: 6, Title: "Hook Prompt", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc",
		AC: []ticket.AcceptanceCriterion{{Description: "Manual check"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()
	r, w, _ := os.Pipe()
	_, _ = w.WriteString("n\n")
	_ = w.Close()
	os.Stdin = r
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"__hook-ac-check", "TKT-006"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected hook prompt denial to fail")
	}

	// Retry with yes.
	r2, w2, _ := os.Pipe()
	_, _ = w2.WriteString("y\n")
	_ = w2.Close()
	os.Stdin = r2
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"__hook-ac-check", "TKT-006"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected hook prompt approval to pass, got: %v", err)
	}
	tk, _ := s.GetTicket(context.Background(), "TKT-006")
	if !tk.AC[0].Done {
		t.Fatal("expected AC to be marked done after y confirmation")
	}
}
