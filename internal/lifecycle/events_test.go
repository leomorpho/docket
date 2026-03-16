package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateEventSchemasAndRequiredFields(t *testing.T) {
	now := time.Date(2026, 3, 16, 16, 30, 0, 0, time.UTC)
	validCases := []Event{
		{
			Version:   SchemaVersionV1,
			Type:      EventRunStart,
			EmittedAt: now.Format(time.RFC3339Nano),
			Payload: map[string]any{
				"run_id":    "run-1",
				"command":   "start",
				"repo_root": "/tmp/repo",
				"ticket_id": "TKT-221",
				"actor":     "agent:test",
			},
		},
		{
			Version:   SchemaVersionV1,
			Type:      EventPhaseEnd,
			EmittedAt: now.Format(time.RFC3339Nano),
			Payload: map[string]any{
				"run_id":  "run-1",
				"phase":   "start.workflow",
				"status":  StatusOK,
				"command": "start",
			},
		},
		{
			Version:   SchemaVersionV1,
			Type:      EventToolFailure,
			EmittedAt: now.Format(time.RFC3339Nano),
			Payload: map[string]any{
				"run_id":  "run-1",
				"phase":   "ac.check",
				"tool":    "ac.run",
				"error":   "exit code 1",
				"command": "__hook-ac-check",
			},
		},
		{
			Version:   SchemaVersionV1,
			Type:      EventRunEnd,
			EmittedAt: now.Format(time.RFC3339Nano),
			Payload: map[string]any{
				"run_id":  "run-1",
				"status":  StatusFailed,
				"command": "__hook-ac-check",
			},
		},
	}

	for _, ev := range validCases {
		report := ValidateEvent(ev)
		if !report.Valid() {
			t.Fatalf("expected valid report for %s, got %#v", ev.Type, report.Errors)
		}
	}

	invalidCases := []struct {
		name string
		ev   Event
		path string
		code string
	}{
		{
			name: "run.start missing run_id",
			ev: Event{
				Version:   SchemaVersionV1,
				Type:      EventRunStart,
				EmittedAt: now.Format(time.RFC3339Nano),
				Payload: map[string]any{
					"command":   "start",
					"repo_root": "/tmp/repo",
				},
			},
			path: "payload.run_id",
			code: CodeRequired,
		},
		{
			name: "phase.end invalid status",
			ev: Event{
				Version:   SchemaVersionV1,
				Type:      EventPhaseEnd,
				EmittedAt: now.Format(time.RFC3339Nano),
				Payload: map[string]any{
					"run_id": "run-1",
					"phase":  "start.workflow",
					"status": "unknown",
				},
			},
			path: "payload.status",
			code: CodeInvalidValue,
		},
		{
			name: "tool.failure missing error",
			ev: Event{
				Version:   SchemaVersionV1,
				Type:      EventToolFailure,
				EmittedAt: now.Format(time.RFC3339Nano),
				Payload: map[string]any{
					"run_id": "run-1",
					"phase":  "ac.check",
					"tool":   "ac.run",
				},
			},
			path: "payload.error",
			code: CodeRequired,
		},
		{
			name: "run.end missing status",
			ev: Event{
				Version:   SchemaVersionV1,
				Type:      EventRunEnd,
				EmittedAt: now.Format(time.RFC3339Nano),
				Payload: map[string]any{
					"run_id": "run-1",
				},
			},
			path: "payload.status",
			code: CodeRequired,
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			report := ValidateEvent(tc.ev)
			if report.Valid() {
				t.Fatal("expected validation errors")
			}
			assertHasError(t, report, tc.path, tc.code)
		})
	}
}

func TestRecorderWritesAndLoadsLifecycleEvents(t *testing.T) {
	repo := t.TempDir()
	now := time.Date(2026, 3, 16, 16, 35, 0, 0, time.UTC)
	clock := fixedClock(now)

	rec, err := StartRun(RunInput{
		RepoRoot: repo,
		Command:  "start",
		TicketID: "TKT-221",
		Actor:    "agent:test",
		Now:      clock,
		RunID:    "run-fixed",
	})
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}
	if err := rec.PhaseEnd("start.workflow", StatusOK); err != nil {
		t.Fatalf("PhaseEnd failed: %v", err)
	}
	if err := rec.End(StatusOK); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	events, err := Load(repo)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != EventRunStart || events[1].Type != EventPhaseEnd || events[2].Type != EventRunEnd {
		t.Fatalf("unexpected event order: %#v", []string{events[0].Type, events[1].Type, events[2].Type})
	}
	for i, ev := range events {
		report := ValidateEvent(ev)
		if !report.Valid() {
			t.Fatalf("event %d failed validation: %#v", i, report.Errors)
		}
	}

	raw, err := os.ReadFile(LogPath(repo))
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 jsonl lines, got %d", len(lines))
	}
	var parsed Event
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("first line must be valid json: %v", err)
	}
	if parsed.Version != SchemaVersionV1 {
		t.Fatalf("expected schema version %q, got %q", SchemaVersionV1, parsed.Version)
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

func assertHasError(t *testing.T, report ValidationReport, path, code string) {
	t.Helper()
	for _, e := range report.Errors {
		if e.Path == path && e.Code == code {
			return
		}
	}
	t.Fatalf("expected error path=%q code=%q, got %#v", path, code, report.Errors)
}

func TestLogPathUnderRuntimeDir(t *testing.T) {
	repo := "/tmp/repo"
	got := LogPath(repo)
	want := filepath.Join(repo, ".docket", "runtime", "lifecycle-events.jsonl")
	if got != want {
		t.Fatalf("expected log path %q, got %q", want, got)
	}
}
