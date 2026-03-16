package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/learning"
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
	h.seedTicket("TKT-960", 960, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})
	h.seedTicket("TKT-961", 961, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "parser reliability and testing coverage"}})

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
	if _, err := store.IngestText("session:TKT-961", `
LEARN[parser]: preserve nested markdown headings.
LEARN[reliability]: retry sqlite busy write paths.
LEARN[testing]: deterministic integration fixtures for parser.
LEARN[ui]: use denser sidebar spacing.
`); err != nil {
		t.Fatalf("seed learn store failed: %v", err)
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
