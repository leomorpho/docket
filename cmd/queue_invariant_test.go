package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func seedInvariantTicket(t *testing.T, repoRoot, id string, seq int, state ticket.State, blockedBy []string) {
	t.Helper()
	s := local.New(repoRoot)
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          id,
		Seq:         seq,
		Title:       "Invariant " + id,
		State:       state,
		Priority:    1,
		Labels:      []string{"topo:leaf"},
		BlockedBy:   append([]string(nil), blockedBy...),
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		Description: "invariant test ticket description",
		AC:          []ticket.AcceptanceCriterion{{Description: "ac"}},
	}); err != nil {
		t.Fatalf("seed %s failed: %v", id, err)
	}
}

func TestUpdateRejectsEmptyStartableLeafInvariant(t *testing.T) {
	h := newFakeRepoHarness(t)
	seedInvariantTicket(t, h.repo, "TKT-101", 101, ticket.State("todo"), nil)
	seedInvariantTicket(t, h.repo, "TKT-102", 102, ticket.State("todo"), []string{"TKT-101"})

	out, err := h.run("update", "TKT-101", "--state", "in-progress")
	if err == nil {
		t.Fatalf("expected invariant rejection, got success output:\n%s", out)
	}
	if !strings.Contains(out, "Queue invariant violated") {
		t.Fatalf("expected queue invariant guidance, got:\n%s", out)
	}
}

func TestUpdateAllowsEmptyStartableLeafWithOverride(t *testing.T) {
	h := newFakeRepoHarness(t)
	seedInvariantTicket(t, h.repo, "TKT-111", 111, ticket.State("todo"), nil)
	seedInvariantTicket(t, h.repo, "TKT-112", 112, ticket.State("todo"), []string{"TKT-111"})

	out, err := h.run("update", "TKT-111", "--state", "in-progress", "--allow-empty-startable-leaf")
	if err != nil {
		t.Fatalf("expected override success, err=%v output=%s", err, out)
	}
}

func TestDoctorReportsQueueInvariantFailure(t *testing.T) {
	h := newFakeRepoHarness(t)
	seedInvariantTicket(t, h.repo, "TKT-121", 121, ticket.State("in-progress"), nil)
	seedInvariantTicket(t, h.repo, "TKT-122", 122, ticket.State("todo"), []string{"TKT-121"})

	out, err := h.run("doctor", "--format", "json")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	var payload doctorReport
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal doctor payload failed: %v\n%s", err, out)
	}
	if statusByName(payload.Checks, "queue_invariant") != "FAIL" {
		t.Fatalf("expected queue_invariant FAIL, got %#v", payload.Checks)
	}
}
