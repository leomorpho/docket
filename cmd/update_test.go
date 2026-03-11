package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestUpdateCmd(t *testing.T) {
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
		Title:       "Original Title",
		State:       ticket.State("backlog"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{}},
	}
	s.CreateTicket(ctx, tick)

	// 1. Update state (backlog -> todo)
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)

	updateState = "todo"
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "todo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update state failed: %v", err)
	}
	if !strings.Contains(b.String(), "state backlog → todo") {
		t.Errorf("expected state transition message, got: %s", b.String())
	}

	updated, _ := s.GetTicket(ctx, "TKT-001")
	if updated.State != ticket.State("todo") {
		t.Errorf("expected state todo, got %s", updated.State)
	}

	// 2. Invalid transition (todo -> done)
	updateState = "done"
	rootCmd.SetArgs([]string{"update", "TKT-001", "--state", "done"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("expected error for invalid transition todo -> done, got nil")
	}

	// 3. Labels and Blockers
	b.Reset()
	// Reset state flag manually since we use global variables and cmd.Flags().Changed persists
	updateState = ""
	updateAddLabels = []string{"feat"}
	updateBlockedBy = []string{"TKT-002"}
	rootCmd.SetArgs([]string{"update", "TKT-001", "--add-label", "feat", "--blocked-by", "TKT-002"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update labels/blockers failed: %v", err)
	}
	updated, _ = s.GetTicket(ctx, "TKT-001")
	if len(updated.Labels) != 1 || updated.Labels[0] != "feat" {
		t.Errorf("Labels mismatch: %v", updated.Labels)
	}
	if len(updated.BlockedBy) != 1 || updated.BlockedBy[0] != "TKT-002" {
		t.Errorf("BlockedBy mismatch: %v", updated.BlockedBy)
	}

	// 4. JSON output
	format = "json"
	b.Reset()
	rootCmd.SetArgs([]string{"update", "TKT-001", "--priority", "2"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("JSON update failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["priority"].(float64) != 2 {
		t.Errorf("expected priority 2 in JSON, got: %v", res["priority"])
	}
}

func TestUpdateCmd_Handoff(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Needs Handoff",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	handoff := "**Current state:**\nDone.\n\n**Decisions made:**\nNone.\n\n**Files touched:**\n- x\n\n**Remaining work:**\n- y\n\n**AC status:**\n- z"

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"update", "TKT-001", "--handoff", handoff})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update handoff failed: %v", err)
	}

	updated, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if updated.Handoff != handoff {
		t.Fatalf("handoff mismatch:\n%s", updated.Handoff)
	}
}

func TestUpdateCmd_HandoffFromStdin(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	s := local.New(tmpDir)
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	tick := &ticket.Ticket{
		ID:          "TKT-001",
		Title:       "Needs Stdin Handoff",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "me",
		Description: "D",
		AC:          []ticket.AcceptanceCriterion{{Description: "A"}},
	}
	if err := s.CreateTicket(ctx, tick); err != nil {
		t.Fatalf("CreateTicket failed: %v", err)
	}

	handoff := "stdin handoff body\nwith two lines"
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	if _, err := w.WriteString(handoff); err != nil {
		t.Fatalf("write pipe failed: %v", err)
	}
	w.Close()
	os.Stdin = r

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-001", "--handoff", "-"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update handoff from stdin failed: %v", err)
	}

	updated, err := s.GetTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("GetTicket failed: %v", err)
	}
	if updated.Handoff != handoff {
		t.Fatalf("handoff mismatch:\n%s", updated.Handoff)
	}
}
