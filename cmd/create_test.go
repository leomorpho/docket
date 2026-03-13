package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestCreateCmd(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"

	// 0. docket init first
	s := local.New(tmpDir)
	cfg := ticket.DefaultConfig()
	ticket.SaveConfig(tmpDir, cfg)

	// 1. Create first ticket
	b := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"create", "--title", "First Ticket", "--priority", "1", "--desc", "This ticket has enough context to satisfy create validation requirements.", "--ac", "A first concrete outcome"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if !strings.Contains(b.String(), "Created TKT-001") {
		t.Errorf("expected TKT-001 created, got: %s", b.String())
	}

	// 2. Verify file exists and has content
	path := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("TKT-001.md not created")
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "title: First Ticket") {
		// Note: render() might use a slightly different YAML format, but let's check title in body too
		if !strings.Contains(string(data), "# TKT-001: First Ticket") {
			t.Errorf("ticket file does not contain title: %s", string(data))
		}
	}
	t1, _ := s.GetTicket(context.Background(), "TKT-001")
	if len(t1.AC) != 1 || t1.AC[0].Description != "A first concrete outcome" {
		t.Fatalf("expected inline AC to be created, got %+v", t1.AC)
	}

	// 3. Create second ticket with DOCKET_ACTOR
	os.Setenv("DOCKET_ACTOR", "agent:test-model")
	defer os.Unsetenv("DOCKET_ACTOR")

	b.Reset()
	errBuf.Reset()
	rootCmd.SetArgs([]string{"create", "--title", "Second Ticket", "--labels", "feat,llm", "--desc", "This second ticket also provides enough context for autonomous execution by another agent."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if !strings.Contains(b.String(), "Created TKT-002") {
		t.Errorf("expected TKT-002 created, got: %s", b.String())
	}

	t2, _ := s.GetTicket(context.Background(), "TKT-002")
	if t2.CreatedBy != "agent:test-model" {
		t.Errorf("CreatedBy mismatch: %s", t2.CreatedBy)
	}
	if len(t2.Labels) != 2 || t2.Labels[0] != "feat" {
		t.Errorf("Labels mismatch: %v", t2.Labels)
	}

	// 4. JSON output
	format = "json"
	b.Reset()
	rootCmd.SetArgs([]string{"create", "--title", "JSON Ticket", "--desc", "JSON output ticket with detailed context for autonomous execution and validation coverage."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("JSON create failed: %v", err)
	}
	var res map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res["id"] != "TKT-003" {
		t.Errorf("expected TKT-003 in JSON, got: %v", res["id"])
	}
}

func TestCreateCmd_ValidationError(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)

	// Reset flags because they are global
	title = ""

	rootCmd.SetArgs([]string{"create"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing title, got nil")
	}
	if !strings.Contains(err.Error(), "--title is required") {
		t.Errorf("expected '--title is required' error, got: %v", err)
	}

	// Test invalid state
	rootCmd.SetArgs([]string{"create", "--title", "T", "--desc", "This description satisfies minimum required context for create validation.", "--state", "invalid"})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid state, got nil")
	}
}

func TestCreateCmd_RequiresDescription(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"create", "--title", "Missing Desc"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--desc is required") {
		t.Fatalf("expected description required error, got %v", err)
	}
}

func TestCreateCmd_WarnsOnShortDescription(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)
	rootCmd.SetArgs([]string{"create", "--title", "Short desc", "--desc", "too short description"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(errOut.String(), "under 50 characters") {
		t.Fatalf("expected short-description warning, got: %s", errOut.String())
	}
}

func TestCreateCmd_ReadyContractHints(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)
	rootCmd.SetArgs([]string{"create", "--title", "Needs guidance", "--desc", "This description is intentionally long enough to avoid the short-description warning but omits readiness sections for guidance checks."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	got := strings.ToLower(errOut.String())
	if !strings.Contains(got, "likely paths") || !strings.Contains(got, "verify commands") || !strings.Contains(got, "out of scope") {
		t.Fatalf("expected ready-contract guidance warnings, got: %s", errOut.String())
	}

	errOut.Reset()
	rootCmd.SetArgs([]string{"create", "--title", "Has guidance", "--desc", "Likely paths: cmd/create.go.\nVerify commands: go test ./cmd/...\nOut of scope: scheduler behavior.\nAdditional implementation context to satisfy authoring expectations."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed with guidance sections: %v", err)
	}
	got = strings.ToLower(errOut.String())
	if strings.Contains(got, "likely paths") || strings.Contains(got, "verify commands") || strings.Contains(got, "out of scope") {
		t.Fatalf("did not expect guidance warnings when sections are present, got: %s", errOut.String())
	}
}

func TestCreateCmd_AutoInjectACDefaultsTypescript(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"dependencies":{"typescript":"^5.0.0"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"create", "--title", "TS defaults", "--desc", "Description long enough to satisfy required quality checks during create command execution."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	s := local.New(tmpDir)
	t1, _ := s.GetTicket(context.Background(), "TKT-001")
	if len(t1.AC) < 2 {
		t.Fatalf("expected at least 2 AC defaults, got %d", len(t1.AC))
	}
	if t1.AC[0].Run == "" {
		t.Fatalf("expected runnable defaults, got %+v", t1.AC)
	}
}

func TestCreateCmd_ACDefaultsConfigOverridesBuiltins(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"dependencies":{"typescript":"^5.0.0"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir .docket: %v", err)
	}
	cfgYAML := "ac_defaults:\n  typescript:\n    - desc: \"Only custom AC\"\n      run: \"echo custom\"\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".docket", "config.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"create", "--title", "TS override", "--desc", "Description long enough to satisfy required quality checks during create command execution."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	s := local.New(tmpDir)
	t1, _ := s.GetTicket(context.Background(), "TKT-001")
	if len(t1.AC) != 1 || t1.AC[0].Description != "Only custom AC" || t1.AC[0].Run != "echo custom" {
		t.Fatalf("expected config override defaults only, got %+v", t1.AC)
	}
}

func TestCreateCmd_NoACDefaultsFlagAndUnknownStack(t *testing.T) {
	tmpDir := t.TempDir()
	repo = tmpDir
	format = "human"
	ticket.SaveConfig(tmpDir, ticket.DefaultConfig())

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"create", "--title", "No defaults", "--desc", "Description long enough to satisfy required quality checks during create command execution.", "--no-ac-defaults"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	s := local.New(tmpDir)
	t1, _ := s.GetTicket(context.Background(), "TKT-001")
	if len(t1.AC) != 0 {
		t.Fatalf("expected no AC defaults with --no-ac-defaults, got %+v", t1.AC)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"create", "--title", "Unknown stack", "--desc", "Description long enough to satisfy required quality checks during create command execution."})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("create failed for unknown stack: %v", err)
	}
	t2, _ := s.GetTicket(context.Background(), "TKT-002")
	if len(t2.AC) != 0 {
		t.Fatalf("expected unknown stack to skip defaults, got %+v", t2.AC)
	}
}
