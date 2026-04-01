package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leomorpho/docket/internal/learning"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestLearningCaptureFromSessionAndCommentArtifacts(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-950", 950, ticket.State("draft"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	sessionCorpus := "noise line\nLEARN[reliability]: retry flaky network calls once.\nLEARN parser: tolerate noisy text around markers.\n"
	sessionPath := filepath.Join(h.repo, "session.log")
	if err := os.WriteFile(sessionPath, []byte(sessionCorpus), 0o644); err != nil {
		t.Fatalf("write synthetic session file failed: %v", err)
	}

	if out, err := h.run("session", "attach", "TKT-950", "--file", sessionPath); err != nil {
		t.Fatalf("session attach failed: %v\n%s", err, out)
	}
	commentBody := "context\nLEARN[testing]: assert deterministic ordering in integration harnesses.\n"
	if out, err := h.run("comment", "TKT-950", "--body", commentBody); err != nil {
		t.Fatalf("comment ingestion failed: %v\n%s", err, out)
	}

	snapshot, err := learning.NewStore(h.repo, nil).Load()
	if err != nil {
		t.Fatalf("load learning snapshot failed: %v", err)
	}
	if len(snapshot.Entries) != 3 {
		t.Fatalf("expected 3 captured learn entries, got %d (%#v)", len(snapshot.Entries), snapshot.Entries)
	}

	categories := map[string]bool{}
	for _, entry := range snapshot.Entries {
		categories[entry.Category] = true
	}
	if !categories["reliability"] || !categories["parser"] || !categories["testing"] {
		t.Fatalf("expected captured categories reliability/parser/testing, got %#v", categories)
	}

	snapshotRaw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot failed: %v", err)
	}
	corpusFixture := h.writeFixture(filepath.Join("learning", "parser-corpus.txt"), []byte(sessionCorpus+commentBody))
	snapshotFixture := h.writeFixture(filepath.Join("learning", "stored-entries.json"), append(snapshotRaw, '\n'))
	t.Logf("learning fixtures: %s | %s", corpusFixture, snapshotFixture)
}
