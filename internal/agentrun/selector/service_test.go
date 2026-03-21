package selector

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestServiceNextReturnsHighestPriorityRunnableTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		id       string
		priority int
		state    ticket.State
		blocked  []string
	}{
		{id: "TKT-101", priority: 2, state: ticket.State("todo")},
		{id: "TKT-102", priority: 1, state: ticket.State("todo")},
		{id: "TKT-103", priority: 1, state: ticket.State("todo"), blocked: []string{"TKT-101"}},
	} {
		if err := store.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          tc.id,
			Seq:         100,
			Title:       tc.id,
			State:       tc.state,
			Priority:    tc.priority,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
			BlockedBy:   tc.blocked,
		}); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if !selection.Found || selection.TicketID != "TKT-102" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

func TestServiceNextSkipsClaimedActiveTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		id       string
		priority int
		state    ticket.State
	}{
		{id: "TKT-101", priority: 1, state: ticket.State("in-progress")},
		{id: "TKT-102", priority: 2, state: ticket.State("todo")},
	} {
		if err := store.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          tc.id,
			Seq:         100 + tc.priority,
			Title:       tc.id,
			State:       tc.state,
			Priority:    tc.priority,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		}); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
	}
	if err := claim.Claim(repoRoot, "TKT-101", filepath.Join(repoRoot, "wt-101"), "human:test"); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if !selection.Found || selection.TicketID != "TKT-102" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

func TestServiceNextReleasesStaleClaimForStartableTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-150",
		Seq:         150,
		Title:       "Runnable",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create TKT-150: %v", err)
	}
	if err := claim.Claim(repoRoot, "TKT-150", filepath.Join(repoRoot, "wt-150"), "human:test"); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if !selection.Found || selection.TicketID != "TKT-150" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
	cl, err := claim.GetClaim(repoRoot, "TKT-150")
	if err != nil {
		t.Fatalf("GetClaim() error = %v", err)
	}
	if cl != nil {
		t.Fatalf("expected stale claim to be released, got %#v", cl)
	}
}

func TestServiceNextSkipsCoordinationTickets(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		id       string
		priority int
		title    string
		labels   []string
	}{
		{id: "TKT-201", priority: 1, title: "Program: Wrapper", labels: []string{"program", "topo:coordination"}},
		{id: "TKT-202", priority: 2, title: "Epic: Wrapper", labels: []string{"topo:coordination"}},
		{id: "TKT-203", priority: 3, title: "Actionable leaf", labels: []string{"topo:leaf"}},
	} {
		if err := store.CreateTicket(context.Background(), &ticket.Ticket{
			ID:          tc.id,
			Seq:         200 + tc.priority,
			Title:       tc.title,
			State:       ticket.State("todo"),
			Priority:    tc.priority,
			Labels:      tc.labels,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		}); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if !selection.Found || selection.TicketID != "TKT-203" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
