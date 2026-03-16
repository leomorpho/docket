package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/leomorpho/docket/internal/applyspec"
)

func TestTicketScaffoldOutputMatchesSchemaAndIsStable(t *testing.T) {
	var first bytes.Buffer
	rootCmd.SetOut(&first)
	rootCmd.SetErr(&first)
	rootCmd.SetArgs([]string{"ticket", "scaffold"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ticket scaffold failed: %v\n%s", err, first.String())
	}

	spec, report, err := applyspec.ParseTicketSpec(first.Bytes())
	if err != nil {
		t.Fatalf("ticket scaffold parse failed: %v\n%s", err, first.String())
	}
	if !report.Valid() {
		t.Fatalf("ticket scaffold should be schema-valid, got %#v", report.Errors)
	}
	if spec.Operation != applyspec.OperationCreate {
		t.Fatalf("expected scaffold operation create, got %q", spec.Operation)
	}

	var second bytes.Buffer
	rootCmd.SetOut(&second)
	rootCmd.SetErr(&second)
	rootCmd.SetArgs([]string{"ticket", "scaffold"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ticket scaffold second run failed: %v\n%s", err, second.String())
	}
	if first.String() != second.String() {
		t.Fatalf("ticket scaffold output should be deterministic across runs")
	}
}

func TestBacklogScaffoldOutputMatchesSchemaAndIsStable(t *testing.T) {
	var first bytes.Buffer
	rootCmd.SetOut(&first)
	rootCmd.SetErr(&first)
	rootCmd.SetArgs([]string{"backlog", "scaffold"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("backlog scaffold failed: %v\n%s", err, first.String())
	}

	spec, report, err := applyspec.ParseBacklogSpec(first.Bytes())
	if err != nil {
		t.Fatalf("backlog scaffold parse failed: %v\n%s", err, first.String())
	}
	if !report.Valid() {
		t.Fatalf("backlog scaffold should be schema-valid, got %#v", report.Errors)
	}
	if len(spec.Tickets) == 0 {
		t.Fatalf("expected scaffold to include at least one backlog ticket")
	}

	var payload map[string]any
	if err := json.Unmarshal(first.Bytes(), &payload); err != nil {
		t.Fatalf("backlog scaffold json parse failed: %v", err)
	}
	if payload["version"] != applyspec.SchemaVersionV1 {
		t.Fatalf("expected scaffold version %q, got %v", applyspec.SchemaVersionV1, payload["version"])
	}

	var second bytes.Buffer
	rootCmd.SetOut(&second)
	rootCmd.SetErr(&second)
	rootCmd.SetArgs([]string{"backlog", "scaffold"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("backlog scaffold second run failed: %v\n%s", err, second.String())
	}
	if first.String() != second.String() {
		t.Fatalf("backlog scaffold output should be deterministic across runs")
	}
}
