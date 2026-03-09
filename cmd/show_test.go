package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestShowCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	// 0. Setup
	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Test Ticket",
		State:       ticket.StateTodo,
		Priority:    1,
		Description: "Desc here",
		AC:          []ticket.AcceptanceCriterion{{Description: "AC1", Done: true}},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
	}
	s.CreateTicket(ctx, tick)

	// 1. Human format
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"show", "TKT-001"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-001 · todo") {
		t.Errorf("expected header, got:\n%s", b.String())
	}
	if !strings.Contains(b.String(), "Test Ticket") {
		t.Errorf("expected title, got:\n%s", b.String())
	}

	// 2. MD format
	b.Reset()
	format = "md"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "md"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show md failed: %v", err)
	}
	if !strings.HasPrefix(b.String(), "---") {
		t.Errorf("expected raw MD starting with ---, got:\n%s", b.String())
	}

	// 3. JSON format
	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["id"] != "TKT-001" {
		t.Errorf("expected ID TKT-001, got: %v", res["id"])
	}

	// 4. Context format
	b.Reset()
	format = "context"
	rootCmd.SetArgs([]string{"show", "TKT-001", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show context failed: %v", err)
	}
	if !strings.Contains(b.String(), "TICKET: TKT-001") {
		t.Errorf("expected compact context output, got:\n%s", b.String())
	}

	// 5. Not found
	b.Reset()
	format = "human"
	rootCmd.SetArgs([]string{"show", "TKT-999"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent ticket, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}
