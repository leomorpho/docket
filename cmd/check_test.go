package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestCheckCmd_R001AndR006AndFix(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-8 * 24 * time.Hour)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-002", Seq: 2, Title: "Blocker", State: ticket.State("done"), Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x", Done: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-001", Seq: 1, Title: "Target", State: ticket.State("in-progress"), Priority: 1, BlockedBy: []string{"TKT-002"}, CreatedAt: old, UpdatedAt: old, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x", Done: true}}}); err != nil {
		t.Fatal(err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"check", "TKT-001"})
	err := rootCmd.Execute()
	if !errors.Is(err, errCheckFindings) {
		t.Fatalf("expected findings error, got %v", err)
	}
	if !strings.Contains(b.String(), "R001") || !strings.Contains(b.String(), "R006") {
		t.Fatalf("unexpected output:\n%s", b.String())
	}
	if strings.Contains(b.String(), "Usage:") {
		t.Fatalf("did not expect usage output on check failure:\n%s", b.String())
	}

	b.Reset()
	rootCmd.SetArgs([]string{"check", "TKT-001", "--fix"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("check --fix failed: %v", err)
	}
	if !strings.Contains(b.String(), "R001") {
		t.Fatalf("expected R001 to still be reported after --fix, got:\n%s", b.String())
	}
	updated, _ := s.GetTicket(context.Background(), "TKT-001")
	if len(updated.BlockedBy) != 0 {
		t.Fatalf("expected blocked_by cleared, got %v", updated.BlockedBy)
	}
}

func TestCheckCmd_JSONAndAllClean(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "json"

	s := local.New(tmpDir)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-010", Seq: 10, Title: "Clean", State: ticket.State("todo"), Priority: 2, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x", Done: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-011", Seq: 11, Title: "Done", State: ticket.State("done"), Priority: 2, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "d", AC: []ticket.AcceptanceCriterion{{Description: "x", Done: true}}}); err != nil {
		t.Fatal(err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"check", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("check json failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &payload); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	if payload["checked"] == nil {
		t.Fatalf("missing checked field")
	}
	if payload["checked"].(float64) != 1 {
		t.Fatalf("expected default check scope to exclude done tickets, checked=%v", payload["checked"])
	}

	format = "human"
	b.Reset()
	rootCmd.SetArgs([]string{"check"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("check clean failed: %v", err)
	}
	if !strings.Contains(b.String(), "All 1 tickets look healthy") {
		t.Fatalf("unexpected clean output: %s", b.String())
	}
}
