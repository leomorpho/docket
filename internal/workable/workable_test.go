package workable

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestTicketsReleaseStaleClaimForStartableTicket(t *testing.T) {
	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(repoRoot, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-101",
		Seq:         101,
		Title:       "Runnable",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := claim.Claim(repoRoot, "TKT-101", filepath.Join(repoRoot, "wt-101"), "human:test"); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	got, err := Tickets(context.Background(), s, cfg, store.Filter{})
	if err != nil {
		t.Fatalf("Tickets() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "TKT-101" {
		t.Fatalf("unexpected tickets: %#v", got)
	}
	cl, err := claim.GetClaim(repoRoot, "TKT-101")
	if err != nil {
		t.Fatalf("GetClaim() error = %v", err)
	}
	if cl != nil {
		t.Fatalf("expected stale claim to be released, got %#v", cl)
	}
}

func TestDiagnoseEmptySummarizesBlockedBacklog(t *testing.T) {
	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-101",
			Seq:         101,
			Title:       "Blocker one",
			State:       ticket.State("in-progress"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-102",
			Seq:         102,
			Title:       "Blocked child one",
			State:       ticket.State("todo"),
			Priority:    2,
			BlockedBy:   []string{"TKT-101"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
		{
			ID:          "TKT-103",
			Seq:         103,
			Title:       "Blocked child two",
			State:       ticket.State("todo"),
			Priority:    3,
			BlockedBy:   []string{"TKT-101"},
			CreatedAt:   now.Add(2 * time.Minute),
			UpdatedAt:   now.Add(2 * time.Minute),
			CreatedBy:   "human:test",
			Description: "desc",
			AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
		},
	} {
		if err := s.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	got, err := DiagnoseEmpty(context.Background(), s, ticket.DefaultConfig())
	if err != nil {
		t.Fatalf("DiagnoseEmpty() error = %v", err)
	}
	if got.StartableTickets != 2 || got.BlockedTickets != 2 {
		t.Fatalf("unexpected diagnosis: %#v", got)
	}
	summary := got.Summary()
	if !strings.Contains(summary, "Queue warning: none are runnable right now") {
		t.Fatalf("expected backlog warning summary, got %q", summary)
	}
	if !strings.Contains(summary, "Top unresolved blockers: TKT-101 x2") {
		t.Fatalf("expected top blocker in summary, got %q", summary)
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
