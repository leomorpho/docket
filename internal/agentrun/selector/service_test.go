package selector

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func runnableSelectorDescription() string {
	return "Implement the selector test fixture with explicit execution context so startable tickets satisfy the runnable contract.\n\nLikely paths:\n- internal/agentrun/selector/service.go\n- internal/workable/diagnosis.go\n\nOut of scope:\n- changing workflow semantics\n- editing unrelated tickets\n\nVerify commands:\n- go test ./internal/agentrun/selector\n- go test ./internal/workable"
}

func runnableSelectorAC() []ticket.AcceptanceCriterion {
	return []ticket.AcceptanceCriterion{
		{Description: "selector path remains covered", Run: "go test ./internal/agentrun/selector", VerificationSteps: []string{"Confirm selector tests still pass."}},
		{Description: "workable diagnosis remains covered", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm workable diagnosis tests still pass."}},
	}
}

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
		{id: "TKT-101", priority: 2, state: ticket.State("ready")},
		{id: "TKT-102", priority: 1, state: ticket.State("ready")},
		{id: "TKT-103", priority: 1, state: ticket.State("ready"), blocked: []string{"TKT-101"}},
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
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
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
		{id: "TKT-101", priority: 1, state: ticket.State("running")},
		{id: "TKT-102", priority: 2, state: ticket.State("ready")},
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
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
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
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: runnableSelectorDescription(),
		AC:          runnableSelectorAC(),
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
			State:       ticket.State("ready"),
			Priority:    tc.priority,
			Labels:      tc.labels,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
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

func TestServiceNextRejectsUngroomedReadyTicket(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-204",
		Seq:         204,
		Title:       "Ungroomed ready ticket",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Short description without runnable sections.",
		AC:          []ticket.AcceptanceCriterion{{Description: "Only one AC"}},
	}); err != nil {
		t.Fatalf("create TKT-204: %v", err)
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if selection.Found {
		t.Fatalf("expected no runnable selection, got %#v", selection)
	}
	if !strings.Contains(selection.Reason, "ready contract is incomplete") {
		t.Fatalf("expected ready-contract diagnosis in selection reason, got %q", selection.Reason)
	}
}

func TestServiceNextSkipsNonLeafParents(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-211",
			Seq:         211,
			Title:       "Parent ticket",
			State:       ticket.State("ready"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
		},
		{
			ID:          "TKT-212",
			Seq:         212,
			Title:       "Child ticket",
			Parent:      "TKT-211",
			State:       ticket.State("ready"),
			Priority:    2,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
		},
	} {
		if err := store.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if !selection.Found || selection.TicketID != "TKT-212" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

func TestServiceNextExplainsBlockedBacklogWhenNothingRunnable(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-301",
			Seq:         301,
			Title:       "Current work",
			State:       ticket.State("running"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
		},
		{
			ID:          "TKT-302",
			Seq:         302,
			Title:       "Blocked backlog",
			State:       ticket.State("ready"),
			Priority:    2,
			BlockedBy:   []string{"TKT-301"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "human:test",
			Description: runnableSelectorDescription(),
			AC:          runnableSelectorAC(),
		},
	} {
		if err := store.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	selection, err := New(Dependencies{Store: store}).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if selection.Found {
		t.Fatalf("expected no runnable selection, got %#v", selection)
	}
	if !strings.HasPrefix(selection.Reason, "no runnable tickets remain:") {
		t.Fatalf("expected explanatory no-runnable reason, got %q", selection.Reason)
	}
	if !strings.Contains(selection.Reason, "TKT-301 x1") {
		t.Fatalf("expected blocker detail in reason, got %q", selection.Reason)
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
