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
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Implement the runnable workable ticket with enough execution context to satisfy the ready contract during stale-claim coverage.\n\nLikely paths:\n- internal/workable/workable.go\n- internal/workable/workable_test.go\n\nOut of scope:\n- unrelated queue migration work\n\nVerify commands:\n- go test ./internal/workable",
		AC: []ticket.AcceptanceCriterion{
			{Description: "stale claims release correctly", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm the stale-claim test passes."}},
			{Description: "ready selection still returns the ticket", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm runnable selection still includes the ticket."}},
		},
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

func TestTicketsSkipUngroomedStartableLeaf(t *testing.T) {
	repoRoot := t.TempDir()
	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-111",
		Seq:         111,
		Title:       "Ungroomed ready ticket",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Short description without runnable sections.",
		AC:          []ticket.AcceptanceCriterion{{Description: "Only one AC"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	got, err := Tickets(context.Background(), s, ticket.DefaultConfig(), store.Filter{})
	if err != nil {
		t.Fatalf("Tickets() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected ungroomed ready ticket to be skipped, got %#v", got)
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
			State:       ticket.State("running"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "human:test",
			Description: "Implement blocker coverage for workable diagnosis with enough execution context to satisfy the ready contract while the blocker itself stays active.\n\nLikely paths:\n- internal/workable/diagnosis.go\n- internal/workable/workable_test.go\n\nOut of scope:\n- unrelated scheduler work\n\nVerify commands:\n- go test ./internal/workable",
			AC: []ticket.AcceptanceCriterion{
				{Description: "active blocker remains addressable", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm diagnosis tests still pass."}},
				{Description: "blocker relationships remain visible", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm blocker summaries still render."}},
			},
		},
		{
			ID:          "TKT-102",
			Seq:         102,
			Title:       "Blocked child one",
			State:       ticket.State("ready"),
			Priority:    2,
			BlockedBy:   []string{"TKT-101"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "human:test",
			Description: "Implement blocked child diagnosis coverage with explicit execution context so the ready contract remains satisfied.\n\nLikely paths:\n- internal/workable/diagnosis.go\n- internal/workable/workable_test.go\n\nOut of scope:\n- unrelated ticket taxonomy changes\n\nVerify commands:\n- go test ./internal/workable",
			AC: []ticket.AcceptanceCriterion{
				{Description: "blocked child remains counted", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm blocked child diagnosis still passes."}},
				{Description: "top blockers remain visible", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm blocker counts still render."}},
			},
		},
		{
			ID:          "TKT-103",
			Seq:         103,
			Title:       "Blocked child two",
			State:       ticket.State("ready"),
			Priority:    3,
			BlockedBy:   []string{"TKT-101"},
			CreatedAt:   now.Add(2 * time.Minute),
			UpdatedAt:   now.Add(2 * time.Minute),
			CreatedBy:   "human:test",
			Description: "Implement second blocked child diagnosis coverage with explicit execution context so the ready contract remains satisfied.\n\nLikely paths:\n- internal/workable/diagnosis.go\n- internal/workable/workable_test.go\n\nOut of scope:\n- unrelated ticket taxonomy changes\n\nVerify commands:\n- go test ./internal/workable",
			AC: []ticket.AcceptanceCriterion{
				{Description: "second blocked child remains counted", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm blocked child diagnosis still passes."}},
				{Description: "shared blocker remains top-ranked", Run: "go test ./internal/workable", VerificationSteps: []string{"Confirm blocker aggregation still renders."}},
			},
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

func TestDiagnoseEmptyReportsUngroomedReadyTickets(t *testing.T) {
	repoRoot := t.TempDir()
	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(repoRoot, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-121",
		Seq:         121,
		Title:       "Ungroomed ready ticket",
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "Short description without runnable sections.",
		AC:          []ticket.AcceptanceCriterion{{Description: "Only one AC"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	got, err := DiagnoseEmpty(context.Background(), s, cfg)
	if err != nil {
		t.Fatalf("DiagnoseEmpty() error = %v", err)
	}
	if got.UngroomedTickets != 1 {
		t.Fatalf("expected one ungroomed ready ticket, got %#v", got)
	}
	summary := got.Summary()
	if !strings.Contains(summary, "ready contract is incomplete") {
		t.Fatalf("expected ready-contract summary, got %q", summary)
	}
	if !strings.Contains(summary, "Groom ready tickets") {
		t.Fatalf("expected grooming guidance in summary, got %q", summary)
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
