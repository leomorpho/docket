package workable

import (
	"context"
	"os/exec"
	"path/filepath"
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

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
