package cmd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/learning"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestBuildLearnReplayRankingAndDeterministicTieBreaking(t *testing.T) {
	repoRoot := t.TempDir()
	now := fixedLearnClock(time.Date(2026, 3, 16, 17, 0, 0, 0, time.UTC))
	store := learning.NewStore(repoRoot, now)
	_, err := store.IngestText("session:TKT-224", `
LEARN[reliability]: parser retries sqlite busy failures.
LEARN[parser]: parser preserves nested headings.
LEARN[testing]: coverage assertions remain stable.
LEARN[ui]: use stronger contrast in dashboard.
`)
	if err != nil {
		t.Fatalf("seed learning store failed: %v", err)
	}

	tkt := &ticket.Ticket{
		ID:          "TKT-224",
		Title:       "Improve parser reliability for ticket start replay",
		Description: "Parser output should remain deterministic in tests.",
		Labels:      []string{"parser", "reliability"},
		AC: []ticket.AcceptanceCriterion{
			{Description: "coverage remains deterministic"},
		},
	}
	got := buildLearnReplay(repoRoot, tkt, 3)
	if len(got) != 3 {
		t.Fatalf("expected top 3 learn rules, got %d (%#v)", len(got), got)
	}
	if got[0].Category != "parser" {
		t.Fatalf("expected deterministic tie-break to prioritize parser category first, got %#v", got)
	}
	if got[1].Category != "reliability" || got[2].Category != "testing" {
		t.Fatalf("expected top categories parser,reliability,testing, got %#v", got)
	}
}

func TestBuildLearnReplayNoDataBehavior(t *testing.T) {
	got := buildLearnReplay(t.TempDir(), &ticket.Ticket{ID: "TKT-001", Title: "noop"}, 3)
	if len(got) != 0 {
		t.Fatalf("expected no learn replay when no store data exists, got %#v", got)
	}
}

func TestStartOutputIncludesTop3LearnRulesAndSnapshots(t *testing.T) {
	h := newFakeRepoHarness(t)
	s := local.New(h.repo)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []*ticket.Ticket{
		{
			ID:          "TKT-960",
			Seq:         960,
			Title:       "Improve parser reliability during start replay",
			State:       ticket.State("ready"),
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:harness-agent",
			Labels:      []string{"parser", "reliability"},
			Description: "Improve parser reliability for start replay with enough execution context to satisfy the ready contract.\n\nLikely paths:\n- cmd/start_learning.go\n- cmd/start_learning_test.go\n\nOut of scope:\n- unrelated UI work\n\nVerify commands:\n- go test ./cmd -run TestStartOutputIncludesTop3LearnRulesAndSnapshots",
			AC:          updateRunnableAC(),
		},
		{
			ID:          "TKT-961",
			Seq:         961,
			Title:       "Parser testing coverage follow-up",
			State:       ticket.State("ready"),
			Priority:    2,
			CreatedAt:   now.Add(time.Second),
			UpdatedAt:   now.Add(time.Second),
			CreatedBy:   "agent:harness-agent",
			Labels:      []string{"parser", "testing"},
			Description: "Improve parser testing coverage for start replay with enough execution context to satisfy the ready contract.\n\nLikely paths:\n- cmd/start_learning.go\n- cmd/start_learning_test.go\n\nOut of scope:\n- unrelated UI work\n\nVerify commands:\n- go test ./cmd -run TestStartOutputIncludesTop3LearnRulesAndSnapshots",
			AC:          updateRunnableAC(),
		},
	} {
		if err := s.CreateTicket(context.Background(), tk); err != nil {
			t.Fatalf("seed ticket %s failed: %v", tk.ID, err)
		}
	}

	beforeOut, err := h.run("start", "--format", "json")
	if err != nil {
		t.Fatalf("start before learn seed failed: %v\n%s", err, beforeOut)
	}
	var before map[string]any
	if err := json.Unmarshal([]byte(beforeOut), &before); err != nil {
		t.Fatalf("unmarshal before start output failed: %v\n%s", err, beforeOut)
	}
	if replay, ok := before["learn_replay"].([]any); ok && len(replay) != 0 {
		t.Fatalf("expected empty learn replay before seed, got %#v", replay)
	}

	store := learning.NewStore(h.repo, fixedLearnClock(time.Date(2026, 3, 16, 17, 10, 0, 0, time.UTC)))
	if _, err := store.IngestText("session:TKT-960", `
LEARN[parser]: preserve nested markdown headings.
LEARN[reliability]: retry sqlite busy write paths.
LEARN[testing]: deterministic integration fixtures for parser.
LEARN[ui]: use denser sidebar spacing.
`); err != nil {
		t.Fatalf("seed learn store failed: %v", err)
	}
	if _, err := store.IngestText("session:TKT-961", `
LEARN[parser]: preserve nested markdown headings.
LEARN[reliability]: retry sqlite busy write paths.
LEARN[testing]: deterministic integration fixtures for parser.
LEARN[ui]: use denser sidebar spacing.
`); err != nil {
		t.Fatalf("seed secondary learn store failed: %v", err)
	}

	afterOut, err := h.run("start", "--format", "json")
	if err != nil {
		t.Fatalf("start after learn seed failed: %v\n%s", err, afterOut)
	}
	var after map[string]any
	if err := json.Unmarshal([]byte(afterOut), &after); err != nil {
		t.Fatalf("unmarshal after start output failed: %v\n%s", err, afterOut)
	}
	replay, ok := after["learn_replay"].([]any)
	if !ok || len(replay) != 3 {
		t.Fatalf("expected top 3 learn replay rules in start output, got %#v", after["learn_replay"])
	}

	beforeFixture := h.writeFixture("learning/start-before.json", []byte(beforeOut))
	afterFixture := h.writeFixture("learning/start-after.json", []byte(afterOut))
	t.Logf("learning start snapshots: %s | %s", beforeFixture, afterFixture)
}

func fixedLearnClock(start time.Time) func() time.Time {
	next := start
	return func() time.Time {
		current := next
		next = next.Add(time.Second)
		return current
	}
}
