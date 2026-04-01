package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestIsAutomationModeFromFlagOrEnv(t *testing.T) {
	prev := automationMode
	defer func() { automationMode = prev }()

	automationMode = false
	t.Setenv("DOCKET_AUTOMATION", "")
	if isAutomationMode() {
		t.Fatal("expected automation mode off by default")
	}

	t.Setenv("DOCKET_AUTOMATION", "1")
	if !isAutomationMode() {
		t.Fatal("expected automation mode on from env")
	}

	t.Setenv("DOCKET_AUTOMATION", "")
	automationMode = true
	if !isAutomationMode() {
		t.Fatal("expected automation mode on from flag")
	}
}

func TestShouldSkipVersionCheckAutomation(t *testing.T) {
	prev := automationMode
	defer func() { automationMode = prev }()
	automationMode = true
	if !shouldSkipVersionCheck(nil) {
		t.Fatal("expected version check to be skipped in automation mode")
	}
}

func TestAutomationModeCoreMutationsDeterministicOutput(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	createOut, createErrOut, err := runRootCommand(t,
		"--automation",
		"--format", "json",
		"create",
		"--title", "Automation create",
		"--desc", "Likely paths: cmd/create.go. Verify commands: go test ./cmd. Out of scope: external orchestration. Additional detail for deterministic automation output tests.",
	)
	if err != nil {
		t.Fatalf("create in automation mode failed: %v", err)
	}
	if strings.TrimSpace(createErrOut) != "" {
		t.Fatalf("expected deterministic empty stderr for create, got: %s", createErrOut)
	}
	var createRes map[string]any
	if err := json.Unmarshal([]byte(createOut), &createRes); err != nil {
		t.Fatalf("parse create output json: %v\noutput=%s", err, createOut)
	}
	if createRes["id"] != "TKT-001" {
		t.Fatalf("unexpected create id: %#v", createRes["id"])
	}

	spec := `{"version":"docket.apply/v1","operation":"create","ticket":{"title":"From automation","description":"Create via ticket apply in automation mode.","blocked_by":["TKT-001"],"ac":["one"]}}`
	specPath := writeSpecFile(t, tmpDir, "automation-ticket.json", spec)
	applyOut, applyErrOut, err := runRootCommand(t,
		"--automation",
		"--format", "json",
		"ticket", "apply",
		"--spec", specPath,
	)
	if err != nil {
		t.Fatalf("ticket apply in automation mode failed: %v", err)
	}
	if strings.TrimSpace(applyErrOut) != "" {
		t.Fatalf("expected deterministic empty stderr for ticket apply, got: %s", applyErrOut)
	}
	var applyRes map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyRes); err != nil {
		t.Fatalf("parse ticket apply output json: %v\noutput=%s", err, applyOut)
	}
	if applyRes["id"] != "TKT-002" {
		t.Fatalf("unexpected ticket apply id: %#v", applyRes["id"])
	}

	path := filepath.Join(tmpDir, ".docket", "tickets", "TKT-002.md")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected ticket artifact %s: %v", path, statErr)
	}
}
