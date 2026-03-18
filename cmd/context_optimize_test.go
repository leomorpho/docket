package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/learning"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestContextOptimizeCmd_JSONIncludesRequiredSources(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "json"
	ticket.SaveConfig(tmp, ticket.DefaultConfig())
	s := local.New(tmp)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-100",
		Seq:         100,
		Title:       "Parent epic",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Parent description",
		AC:          []ticket.AcceptanceCriterion{{Description: "parent ac"}},
	}); err != nil {
		t.Fatalf("create parent ticket: %v", err)
	}
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:            "TKT-270",
		Seq:           270,
		Title:         "Context optimizer",
		State:         ticket.State("in-progress"),
		Priority:      2,
		Parent:        "TKT-100",
		BlockedBy:     []string{"TKT-251"},
		LinkedCommits: []string{"43aac97", "ccb63ce"},
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     "human:test",
		Description:   "Add a context optimizer that outputs compact task briefs for agents.",
		Comments: []ticket.Comment{
			{Author: "human:alice", Body: "Need a deterministic JSON shape for downstream parsers."},
			{Author: "agent:test", Body: "Drafted output sections and limits."},
		},
		AC: []ticket.AcceptanceCriterion{
			{Description: "brief includes related sources", Done: false},
			{Description: "tests verify bounded output", Done: false},
		},
	}); err != nil {
		t.Fatalf("create target ticket: %v", err)
	}
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-271",
		Seq:         271,
		Title:       "Sibling ticket",
		State:       ticket.State("todo"),
		Priority:    2,
		Parent:      "TKT-100",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Sibling description",
		AC:          []ticket.AcceptanceCriterion{{Description: "sibling ac"}},
	}); err != nil {
		t.Fatalf("create sibling ticket: %v", err)
	}

	if _, err := learning.NewStore(tmp, nil).IngestText("session:TKT-270", "LEARN[testing]: keep optimizer output predictable for machine readers."); err != nil {
		t.Fatalf("seed learn store: %v", err)
	}

	if _, err := writeCheckpoint(tmp, checkpoint{
		TicketID:     "TKT-270",
		CreatedAt:    now.Format(time.RFC3339),
		ACDone:       0,
		ACTotal:      2,
		ChangedFiles: []string{"cmd/context_optimize.go"},
		LastComments: []string{"checkpoint context"},
		Branch:       "docket/TKT-270",
		WorktreePath: tmp,
		Summary:      "Handoff summary for resume.",
	}); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"context-optimize", "TKT-270", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("context-optimize failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v\n%s", err, out.String())
	}
	if payload["ticket_id"] != "TKT-270" {
		t.Fatalf("ticket_id = %v, want TKT-270", payload["ticket_id"])
	}
	if strings.TrimSpace(fmt.Sprintf("%v", payload["brief"])) == "" {
		t.Fatalf("expected non-empty brief, got: %+v", payload)
	}
	related, ok := payload["related_work"].([]any)
	if !ok || len(related) == 0 {
		t.Fatalf("expected related_work entries, got: %+v", payload["related_work"])
	}
	learningRules, ok := payload["learning_rules"].([]any)
	if !ok || len(learningRules) == 0 {
		t.Fatalf("expected learning_rules entries, got: %+v", payload["learning_rules"])
	}
	recent, ok := payload["recent_activity"].(map[string]any)
	if !ok {
		t.Fatalf("expected recent_activity object, got: %+v", payload["recent_activity"])
	}
	comments, ok := recent["comments"].([]any)
	if !ok || len(comments) == 0 {
		t.Fatalf("expected recent comments, got: %+v", recent["comments"])
	}
	if strings.TrimSpace(fmt.Sprintf("%v", recent["checkpoint_summary"])) == "" {
		t.Fatalf("expected checkpoint_summary, got: %+v", recent)
	}
	nextSteps, ok := payload["next_steps"].([]any)
	if !ok || len(nextSteps) == 0 {
		t.Fatalf("expected next_steps, got: %+v", payload["next_steps"])
	}
}

func TestContextOptimizeCmd_BoundsOutputShape(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "json"
	ticket.SaveConfig(tmp, ticket.DefaultConfig())
	s := local.New(tmp)
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-110",
		Seq:         110,
		Title:       "Parent",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Parent description",
		AC:          []ticket.AcceptanceCriterion{{Description: "parent ac"}},
	}); err != nil {
		t.Fatalf("create parent: %v", err)
	}

	comments := make([]ticket.Comment, 0, 8)
	for i := 0; i < 8; i++ {
		comments = append(comments, ticket.Comment{Author: "agent:test", Body: fmt.Sprintf("comment %d", i+1)})
	}
	linkedCommits := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		linkedCommits = append(linkedCommits, fmt.Sprintf("sha-%d", i+1))
	}
	ac := make([]ticket.AcceptanceCriterion, 0, 8)
	for i := 0; i < 8; i++ {
		ac = append(ac, ticket.AcceptanceCriterion{Description: fmt.Sprintf("ac item %d", i+1), Done: false})
	}

	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:            "TKT-270",
		Seq:           270,
		Title:         "Optimizer bounded output",
		State:         ticket.State("in-progress"),
		Priority:      1,
		Parent:        "TKT-110",
		LinkedCommits: linkedCommits,
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     "human:test",
		Description:   "Ensure compact output remains bounded across all lists and sections.",
		Comments:      comments,
		AC:            ac,
	}); err != nil {
		t.Fatalf("create target ticket: %v", err)
	}

	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("TKT-%d", 300+i)
		if err := s.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          id,
			Seq:         300 + i,
			Title:       "Sibling " + id,
			State:       ticket.State("todo"),
			Priority:    2,
			Parent:      "TKT-110",
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Sibling description",
			AC:          []ticket.AcceptanceCriterion{{Description: "sibling ac"}},
		}); err != nil {
			t.Fatalf("create sibling %s: %v", id, err)
		}
	}

	var corpus strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&corpus, "LEARN[testing]: optimizer rule %d for bounded output.\n", i+1)
	}
	if _, err := learning.NewStore(tmp, nil).IngestText("session:TKT-270", corpus.String()); err != nil {
		t.Fatalf("seed learn store: %v", err)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"context-optimize", "TKT-270", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("context-optimize failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse json output: %v\n%s", err, out.String())
	}
	related, _ := payload["related_work"].([]any)
	if len(related) > contextOptimizeMaxItems {
		t.Fatalf("related_work length = %d, want <= %d", len(related), contextOptimizeMaxItems)
	}
	learningRules, _ := payload["learning_rules"].([]any)
	if len(learningRules) > contextOptimizeMaxItems {
		t.Fatalf("learning_rules length = %d, want <= %d", len(learningRules), contextOptimizeMaxItems)
	}
	nextSteps, _ := payload["next_steps"].([]any)
	if len(nextSteps) > contextOptimizeMaxItems {
		t.Fatalf("next_steps length = %d, want <= %d", len(nextSteps), contextOptimizeMaxItems)
	}
	recent, _ := payload["recent_activity"].(map[string]any)
	commentsOut, _ := recent["comments"].([]any)
	if len(commentsOut) > contextOptimizeMaxItems {
		t.Fatalf("recent comments length = %d, want <= %d", len(commentsOut), contextOptimizeMaxItems)
	}
	commitsOut, _ := recent["linked_commits"].([]any)
	if len(commitsOut) > contextOptimizeMaxItems {
		t.Fatalf("recent linked_commits length = %d, want <= %d", len(commitsOut), contextOptimizeMaxItems)
	}
}
