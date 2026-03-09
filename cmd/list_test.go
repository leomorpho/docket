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

func TestListCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	// 0. Setup store and tickets
	s := local.New(tmpDir)
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	ctx := context.Background()
	now := time.Now().UTC()

	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-001", Title: "Open Ticket", State: ticket.StateTodo, Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-002", Title: "Done Ticket", State: ticket.StateDone, Priority: 1, CreatedAt: now.Add(time.Hour), UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})
	s.CreateTicket(ctx, &ticket.Ticket{ID: "TKT-003", Title: "Archived Ticket", State: ticket.StateArchived, Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "D", AC: []ticket.AcceptanceCriterion{{}}})

	// 1. Default list (shows only open, which means everything except done/archived by default config, but TASK-008 says "open = all except done/archived")
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"list"})
	listState = "open" // Reset flag
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-001") {
		t.Errorf("expected TKT-001 in default list, got:\n%s", b.String())
	}
	if strings.Contains(b.String(), "TKT-002") || strings.Contains(b.String(), "TKT-003") {
		t.Errorf("expected only open tickets, but got:\n%s", b.String())
	}

	// 2. List done
	b.Reset()
	rootCmd.SetArgs([]string{"list", "--state", "done"})
	listState = "done"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list done failed: %v", err)
	}
	if !strings.Contains(b.String(), "TKT-002") {
		t.Errorf("expected TKT-002 in done list, got:\n%s", b.String())
	}

	// 3. Format context
	b.Reset()
	rootCmd.SetArgs([]string{"list", "--format", "context"})
	format = "context"
	listState = "open"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list context failed: %v", err)
	}
	if !strings.Contains(b.String(), "[TKT-001] P1 todo") {
		t.Errorf("expected compact context line, got:\n%s", b.String())
	}

	// 4. Format JSON
	b.Reset()
	rootCmd.SetArgs([]string{"list", "--format", "json"})
	format = "json"
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	var res []map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("expected 1 open ticket in JSON, got: %d", len(res))
	} else if res[0]["id"] != "TKT-001" {
		t.Errorf("expected TKT-001 in JSON, got: %v", res[0]["id"])
	}
}
