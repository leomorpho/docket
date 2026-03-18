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

func TestLearnCaptureStoresCategorizedRuleWithSourceMetadata(t *testing.T) {
	h := newFakeRepoHarness(t)

	captureOut, err := h.run(
		"learn", "capture",
		"--category", "reliability",
		"--rule", "retry flaky network calls once before failing.",
		"--source", "manual:TKT-267",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("learn capture failed: %v\n%s", err, captureOut)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(captureOut), &payload); err != nil {
		t.Fatalf("unmarshal learn capture json failed: %v\n%s", err, captureOut)
	}
	if payload["added"].(float64) != 1 {
		t.Fatalf("expected added=1 for first capture, got %#v", payload)
	}
	entry := payload["entry"].(map[string]any)
	if entry["category"] != "reliability" || entry["source"] != "manual:TKT-267" {
		t.Fatalf("expected captured category+source metadata, got %#v", entry)
	}

	searchOut, err := h.run("learn", "search", "flaky network", "--format", "json")
	if err != nil {
		t.Fatalf("learn search failed after capture: %v\n%s", err, searchOut)
	}
	var search map[string]any
	if err := json.Unmarshal([]byte(searchOut), &search); err != nil {
		t.Fatalf("unmarshal learn search json failed: %v\n%s", err, searchOut)
	}
	entries := search["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected one captured learn rule in search output, got %#v", search)
	}
}

func TestLearnCaptureDeduplicatesAndValidatesInput(t *testing.T) {
	h := newFakeRepoHarness(t)

	firstOut, err := h.run(
		"learn", "capture",
		"--category", "testing",
		"--rule", "use deterministic fixtures in integration tests.",
		"--source", "manual:first",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("first learn capture failed: %v\n%s", err, firstOut)
	}
	secondOut, err := h.run(
		"learn", "capture",
		"--category", "testing",
		"--rule", "use deterministic fixtures in integration tests.",
		"--source", "manual:second",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("second learn capture failed: %v\n%s", err, secondOut)
	}
	var second map[string]any
	if err := json.Unmarshal([]byte(secondOut), &second); err != nil {
		t.Fatalf("unmarshal second capture json failed: %v\n%s", err, secondOut)
	}
	if second["added"].(float64) != 0 {
		t.Fatalf("expected dedupe to return added=0, got %#v", second)
	}

	listOut, err := h.run("learn", "list", "--format", "json")
	if err != nil {
		t.Fatalf("learn list json failed after dedupe test: %v\n%s", err, listOut)
	}
	var list map[string]any
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("unmarshal list json failed: %v\n%s", err, listOut)
	}
	entries := list["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected dedupe to keep a single learn entry, got %#v", list)
	}

	invalidOut, err := h.run("learn", "capture", "--category", "testing", "--source", "manual:invalid")
	if err == nil {
		t.Fatalf("expected capture without --rule to fail, output=%s", invalidOut)
	}
	if !strings.Contains(invalidOut, "--rule is required") {
		t.Fatalf("expected clear --rule validation error, got:\n%s", invalidOut)
	}
}
