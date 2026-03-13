package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestValidateCmd_PrescriptiveForDirectEdit(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "T",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	edited := strings.Replace(string(raw), "priority: 1", "priority: 2", 1)
	if err := os.WriteFile(p, []byte(edited), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	showWarns = false
	strict = false
	rootCmd.SetArgs([]string{"validate", "TKT-001", "--warn"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected validation failure due to signature mismatch")
	}
	out := b.String()
	if !strings.Contains(out, "accepted schema-valid direct edit") || !strings.Contains(out, "docket update TKT-001 --priority 2") {
		t.Fatalf("expected prescriptive command in validate output, got:\n%s", out)
	}
}

func TestValidateCmd_RevertsInvalidDirectEdit(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "T",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "Description with enough words to satisfy validation checks in this test case.",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}, {Description: "B"}},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	p := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	edited := strings.Replace(string(raw), "state: todo", "state: definitely-invalid", 1)
	if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	showWarns = false
	strict = false
	rootCmd.SetArgs([]string{"validate", "TKT-001"})
	if err := rootCmd.Execute(); err == nil {
		t.Logf("Output: %s", b.String())
		t.Fatal("expected validate to fail once and report rejected invalid direct edit")
	}

	ticketAfter, err := s.GetTicket(context.Background(), "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if ticketAfter.State != ticket.State("todo") {
		t.Fatalf("expected state to be reverted to todo, got %s", ticketAfter.State)
	}
}

func TestValidateCmd_StrictFailsOnWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "T",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "This description is intentionally long enough to avoid short-description warnings in strict mode validation tests.",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}, {Description: "B"}},
		Labels:      []string{},
	}); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	showWarns = false
	strict = false
	rootCmd.SetArgs([]string{"validate", "TKT-001", "--strict"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected strict mode to fail due warnings")
	}
}

func TestValidateAll(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	desc := "Has twenty words or more in the description to pass the quality check easily and without any issues whatsoever."
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "T1", State: "todo", CreatedBy: "me", CreatedAt: now, UpdatedAt: now, Description: desc, AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "T2", State: "todo", CreatedBy: "me", CreatedAt: now, UpdatedAt: now, Description: desc, AC: []ticket.AcceptanceCriterion{{Description: "A"}}})

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	showWarns = false
	strict = false
	rootCmd.SetArgs([]string{"validate"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("validate all failed: %v", err)
	}
	if !strings.Contains(b.String(), "All tickets valid.") {
		t.Errorf("expected clean output, got: %s", b.String())
	}
}
