package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestLinkPersistsAndShowDisplaysRelations(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "context"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-201", Seq: 201, Title: "A", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-202", Seq: 202, Title: "B", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"link", "TKT-201", "TKT-202", "--relation", "blocks"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("link failed: %v", err)
	}
	st, err := loadRelations(tmp)
	if err != nil || len(st.Relations) == 0 {
		t.Fatalf("expected persisted relation, got err=%v state=%+v", err, st)
	}

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"show", "TKT-201", "--format", "context"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(out.String(), "RELATIONS:") {
		t.Fatalf("expected relations in show output, got: %s", out.String())
	}
}

func TestWorktreeStartBlockedByRelationUnlessForce(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-301", Seq: 301, Title: "A", State: "running", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: updateRunnableDescription(), AC: updateRunnableAC()})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-302", Seq: 302, Title: "B", State: "ready", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: updateRunnableDescription(), AC: updateRunnableAC()})
	_ = upsertRelation(tmp, relationEntry{From: "TKT-301", To: "TKT-302", Relation: "blocks"})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"worktree", "start", "TKT-302", filepath.Join(tmp, "wt")})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected blocked relation to prevent worktree start without --force")
	}
}

func TestLinkRejectsParallelSafeRelation(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-401", Seq: 401, Title: "A", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-402", Seq: 402, Title: "B", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"link", "TKT-401", "TKT-402", "--relation", "parallel-safe"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected parallel-safe relation to be rejected")
	}
	st, err := loadRelations(tmp)
	if err != nil {
		t.Fatalf("loadRelations() error = %v", err)
	}
	if len(st.Relations) != 0 {
		t.Fatalf("expected rejected relation to leave no persisted state, got %#v", st.Relations)
	}
}

func TestLinkStillAllowsSupportedRelations(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-501", Seq: 501, Title: "A", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-502", Seq: 502, Title: "B", State: "draft", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"link", "TKT-501", "TKT-502", "--relation", "depends-on"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected depends-on relation to remain supported: %v", err)
	}
	st, err := loadRelations(tmp)
	if err != nil {
		t.Fatalf("loadRelations() error = %v", err)
	}
	if len(st.Relations) != 1 || st.Relations[0].Relation != "depends-on" {
		t.Fatalf("expected supported relation to persist, got %#v", st.Relations)
	}
}
