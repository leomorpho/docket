package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVariantsAndMalformedLines(t *testing.T) {
	text := `
noise line
LEARN: Prefer deterministic fixtures over brittle snapshots.
- LEARN[testing]: Use fixed clocks in integration harnesses.
* LEARN(parser): tolerate noisy surrounding text.
LEARN reliability: retry sqlite busy once before failing
LEARN:
LEARN [ ]: invalid category
`
	parsed := Parse(text)
	if len(parsed) != 4 {
		t.Fatalf("expected 4 parsed rules, got %d (%#v)", len(parsed), parsed)
	}
	if parsed[0].Category != "general" || !strings.Contains(parsed[0].Rule, "deterministic fixtures") {
		t.Fatalf("unexpected first parsed rule: %+v", parsed[0])
	}
	if parsed[1].Category != "testing" {
		t.Fatalf("expected bracket category parsing, got %+v", parsed[1])
	}
	if parsed[2].Category != "parser" {
		t.Fatalf("expected parenthetical category parsing, got %+v", parsed[2])
	}
	if parsed[3].Category != "reliability" {
		t.Fatalf("expected trailing category parsing, got %+v", parsed[3])
	}
}

func TestStoreDedupeAndReadWriteIntegrity(t *testing.T) {
	repo := t.TempDir()
	now := fixedClock(time.Date(2026, 3, 16, 16, 45, 0, 0, time.UTC))
	store := NewStore(repo, now)

	first := `
LEARN[testing]: Use golden files for parser fixtures.
LEARN[testing]: Use golden files for parser fixtures.
LEARN[ci]: Keep tests deterministic.
`
	res, err := store.IngestText("comment:TKT-223", first)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if res.Added != 2 || res.Total != 2 {
		t.Fatalf("expected 2 unique entries after first ingest, got %+v", res)
	}

	second := `
LEARN[ci]: Keep tests deterministic.
LEARN: Add schema validation snapshots.
`
	res, err = store.IngestText("session:TKT-223", second)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if res.Added != 1 || res.Total != 3 {
		t.Fatalf("expected deduped growth to total=3, got %+v", res)
	}

	snap, err := store.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if snap.Version != StoreVersion {
		t.Fatalf("expected snapshot version %d, got %d", StoreVersion, snap.Version)
	}
	if len(snap.Entries) != 3 {
		t.Fatalf("expected 3 stored entries, got %d", len(snap.Entries))
	}
	for _, entry := range snap.Entries {
		if entry.CapturedAt == "" || entry.Source == "" || entry.Rule == "" {
			t.Fatalf("expected complete stored entry fields, got %+v", entry)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".docket", "runtime", "learn-rules.json")); err != nil {
		t.Fatalf("expected persistent store file: %v", err)
	}
}

func fixedClock(start time.Time) func() time.Time {
	next := start
	return func() time.Time {
		current := next
		next = next.Add(time.Second)
		return current
	}
}
