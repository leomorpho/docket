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
		ID:        "TKT-001",
		Title:     "Original Title",
		State:     ticket.StateBacklog,
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "me",
		Description: "D",
		AC: []ticket.AcceptanceCriterion{{}},
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
	if updated.State != ticket.StateTodo {
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
