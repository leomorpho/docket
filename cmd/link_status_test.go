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
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-201", Seq: 201, Title: "A", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-202", Seq: 202, Title: "B", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})

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
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-301", Seq: 301, Title: "A", State: "in-progress", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-302", Seq: 302, Title: "B", State: "todo", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})
	_ = upsertRelation(tmp, relationEntry{From: "TKT-301", To: "TKT-302", Relation: "blocks"})

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"worktree", "start", "TKT-302", filepath.Join(tmp, "wt")})
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("expected blocked relation to prevent worktree start without --force")
	}
}

func TestStatusParallelMatrixUsesRelationsAndLockOverlap(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-401", Seq: 401, Title: "A", State: "in-progress", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-402", Seq: 402, Title: "B", State: "in-progress", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})
	_ = upsertLock(tmp, fileLock{TicketID: "TKT-401", WorktreePath: tmp, Files: []string{"same.go"}, UpdatedAt: now.Format(time.RFC3339)})
	_ = upsertLock(tmp, fileLock{TicketID: "TKT-402", WorktreePath: tmp, Files: []string{"same.go"}, UpdatedAt: now.Format(time.RFC3339)})

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"status", "--parallel"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status --parallel failed: %v", err)
	}
	if !strings.Contains(out.String(), "risky:") {
		t.Fatalf("expected risky matrix indicator, got: %s", out.String())
	}
}

func TestStatusParallelMatrixUsesConfiguredActiveRole(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"coding"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"coding": {
				Label:            "Coding",
				Open:             true,
				Column:           1,
				Next:             []string{"testing"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"testing": {
				Label:            "Testing",
				Open:             true,
				Column:           2,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           3,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   4,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState: "queued",
	}
	_ = ticket.SaveConfig(tmp, cfg)
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-501", Seq: 501, Title: "A", State: "coding", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}}})
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{ID: "TKT-502", Seq: 502, Title: "B", State: "testing", Priority: 1, CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "B"}}})
	_ = upsertLock(tmp, fileLock{TicketID: "TKT-501", WorktreePath: tmp, Files: []string{"same.go"}, UpdatedAt: now.Format(time.RFC3339)})
	_ = upsertLock(tmp, fileLock{TicketID: "TKT-502", WorktreePath: tmp, Files: []string{"same.go"}, UpdatedAt: now.Format(time.RFC3339)})

	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"status", "--parallel"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status --parallel failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-501 <-> TKT-502") {
		t.Fatalf("expected configured active-role tickets in matrix, got: %s", out.String())
	}
}
