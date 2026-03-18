package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/learning"
)

func TestLearnListEmptyStateHumanAndJSON(t *testing.T) {
	h := newFakeRepoHarness(t)

	humanOut, err := h.run("learn", "list")
	if err != nil {
		t.Fatalf("learn list human failed: %v\n%s", err, humanOut)
	}
	if !strings.Contains(humanOut, "No stored learn rules.") {
		t.Fatalf("expected empty-state human output, got:\n%s", humanOut)
	}

	jsonOut, err := h.run("learn", "list", "--format", "json")
	if err != nil {
		t.Fatalf("learn list json failed: %v\n%s", err, jsonOut)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal learn list json failed: %v\n%s", err, jsonOut)
	}
	entries, ok := payload["entries"].([]any)
	if !ok || len(entries) != 0 {
		t.Fatalf("expected empty entries in learn list json, got %#v", payload["entries"])
	}
}

func TestLearnSearchFiltersAndReportsStableFields(t *testing.T) {
	h := newFakeRepoHarness(t)
	store := learning.NewStore(h.repo, fixedLearnClock(time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)))
	if _, err := store.IngestText("session:TKT-265", `
LEARN[parser]: preserve nested markdown headings.
LEARN[testing]: deterministic integration fixtures for parser output.
LEARN[reliability]: retry sqlite busy writes with bounded backoff.
`); err != nil {
		t.Fatalf("seed learn store failed: %v", err)
	}

	searchOut, err := h.run("learn", "search", "sqlite")
	if err != nil {
		t.Fatalf("learn search human failed: %v\n%s", err, searchOut)
	}
	if !strings.Contains(searchOut, "[reliability]") || !strings.Contains(searchOut, "retry sqlite busy writes with bounded backoff") {
		t.Fatalf("expected filtered reliability rule in search output, got:\n%s", searchOut)
	}
	if !strings.Contains(searchOut, "source: session:TKT-265") {
		t.Fatalf("expected source field in human search output, got:\n%s", searchOut)
	}
	if strings.Contains(searchOut, "[testing]") || strings.Contains(searchOut, "[parser]") {
		t.Fatalf("search output should exclude non-matching rules, got:\n%s", searchOut)
	}

	jsonOut, err := h.run("learn", "search", "parser", "--format", "json")
	if err != nil {
		t.Fatalf("learn search json failed: %v\n%s", err, jsonOut)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal learn search json failed: %v\n%s", err, jsonOut)
	}
	if payload["query"] != "parser" {
		t.Fatalf("expected parser query echo, got %#v", payload["query"])
	}
	entries, ok := payload["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("expected 2 parser-related entries, got %#v", payload["entries"])
	}
	first := entries[0].(map[string]any)
	if first["category"] == nil || first["rule"] == nil || first["source"] == nil {
		t.Fatalf("expected category/rule/source fields in learn search json entry, got %#v", first)
	}
}
