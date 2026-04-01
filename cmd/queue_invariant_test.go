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
		Description: "Likely paths: cmd/queue_invariant_test.go and cmd/update.go. Verify commands: go test ./cmd -run QueueInvariant -count=1. Out of scope: unrelated queue healing behavior. This fixture is intentionally detailed enough to satisfy runnable-state requirements while testing queue invariants.",
		AC: []ticket.AcceptanceCriterion{
			{Description: "queue invariant can be evaluated"},
			{Description: "startable leaf accounting remains deterministic"},
		},
	}); err != nil {
		t.Fatalf("seed %s failed: %v", id, err)
	}
}

func TestUpdateRejectsEmptyStartableLeafInvariant(t *testing.T) {
	h := newFakeRepoHarness(t)
	seedInvariantTicket(t, h.repo, "TKT-101", 101, ticket.State("ready"), nil)
	seedInvariantTicket(t, h.repo, "TKT-102", 102, ticket.State("ready"), []string{"TKT-101"})

	out, err := h.run("update", "TKT-101", "--state", "running")
	if err == nil {
		t.Fatalf("expected invariant rejection, got success output:\n%s", out)
	}
	if !strings.Contains(out, "Queue invariant violated") {
		t.Fatalf("expected queue invariant guidance, got:\n%s", out)
	}
}

func TestUpdateAllowsEmptyStartableLeafWithOverride(t *testing.T) {
	h := newFakeRepoHarness(t)
	seedInvariantTicket(t, h.repo, "TKT-111", 111, ticket.State("ready"), nil)
	seedInvariantTicket(t, h.repo, "TKT-112", 112, ticket.State("ready"), []string{"TKT-111"})

	out, err := h.run("update", "TKT-111", "--state", "running", "--allow-empty-startable-leaf")
	if err != nil {
		t.Fatalf("expected override success, err=%v output=%s", err, out)
	}
}

func TestDoctorReportsQueueInvariantFailure(t *testing.T) {
	h := newFakeRepoHarness(t)
	seedInvariantTicket(t, h.repo, "TKT-121", 121, ticket.State("running"), nil)
	seedInvariantTicket(t, h.repo, "TKT-122", 122, ticket.State("ready"), []string{"TKT-121"})

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

func TestDoctorReportsQueueInvariantFailureWithoutTopologyLabels(t *testing.T) {
	h := newFakeRepoHarness(t)
	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)
	for _, item := range []*ticket.Ticket{
		{
			ID:          "TKT-131",
			Seq:         131,
			Title:       "Active blocker without topo labels",
			State:       ticket.State("running"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			Description: "Likely paths: cmd/queue_invariant.go and cmd/doctor.go. Verify commands: go test ./cmd -run QueueInvariant -count=1. Out of scope: topology-label policy. This fixture satisfies the runnable contract except for the unresolved blocker relationship.",
			AC: []ticket.AcceptanceCriterion{
				{Description: "doctor evaluates actual runnable work"},
				{Description: "topology labels are not required for queue truth"},
			},
		},
		{
			ID:          "TKT-132",
			Seq:         132,
			Title:       "Blocked ready leaf without topo labels",
			State:       ticket.State("ready"),
			Priority:    2,
			BlockedBy:   []string{"TKT-131"},
			CreatedAt:   now.Add(time.Minute),
			UpdatedAt:   now.Add(time.Minute),
			CreatedBy:   "agent:test",
			Description: "Likely paths: cmd/queue_invariant.go and internal/workable/diagnosis.go. Verify commands: go test ./cmd -run QueueInvariant -count=1. Out of scope: selector/status alignment. This fixture is intentionally groomed so only the blocker keeps it from being runnable.",
			AC: []ticket.AcceptanceCriterion{
				{Description: "doctor still fails when no runnable work exists"},
				{Description: "diagnosis names the blocking ticket"},
			},
		},
	} {
		item := *item
		if err := s.CreateTicket(context.Background(), &item); err != nil {
			t.Fatalf("seed %s failed: %v", item.ID, err)
		}
	}

	out, err := h.run("doctor", "--format", "json")
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, out)
	}
	var payload doctorReport
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal doctor payload failed: %v\n%s", err, out)
	}
	if statusByName(payload.Checks, "queue_invariant") != "FAIL" {
		t.Fatalf("expected queue_invariant FAIL without topology labels, got %#v", payload.Checks)
	}
	if !strings.Contains(queueTruthDoctorDetail(payload.Checks), "TKT-131") {
		t.Fatalf("expected queue invariant detail to explain the blocker, got %#v", payload.Checks)
	}
}
