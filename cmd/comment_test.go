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

func TestCommentCmd(t *testing.T) {
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
		Title:     "Test Ticket",
		State:     ticket.StateTodo,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "me",
		Description: "D",
		AC: []ticket.AcceptanceCriterion{{}},
	}
	s.CreateTicket(ctx, tick)

	// 1. Add comment via flag
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	
	commentBody = "First comment"
	rootCmd.SetArgs([]string{"comment", "TKT-001", "--body", "First comment"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("comment failed: %v", err)
	}
	if !strings.Contains(b.String(), "Comment added to TKT-001") {
		t.Errorf("expected success message, got: %s", b.String())
	}

	// 2. Verify comment in file
	updated, _ := s.GetTicket(ctx, "TKT-001")
	if len(updated.Comments) != 1 || updated.Comments[0].Body != "First comment" {
		t.Errorf("expected 1 comment, got: %v", updated.Comments)
	}

	// 3. Add second comment via JSON
	format = "json"
	b.Reset()
	commentBody = "Second comment"
	rootCmd.SetArgs([]string{"comment", "TKT-001", "--body", "Second comment", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("JSON comment failed: %v", err)
	}
	var res map[string]string
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["ticket_id"] != "TKT-001" {
		t.Errorf("expected ticket_id TKT-001, got: %v", res["ticket_id"])
	}

	updated, _ = s.GetTicket(ctx, "TKT-001")
	if len(updated.Comments) != 2 {
		t.Errorf("expected 2 comments, got: %d", len(updated.Comments))
	}
}
