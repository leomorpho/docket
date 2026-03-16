package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/capabilities"
)

func TestCapabilitiesCommandJSONIncludesContractAndAdapterMetadata(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_ADAPTER", "")
	docketHome = ""
	repo = tmpRepo
	format = "json"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "doombox.json"), []byte(`{"mcp":"docket"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}
	contract := sampleContract()
	written, _, err := capabilities.WriteRuntimeContract(tmpRepo, contract)
	if err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"capabilities", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("capabilities command failed: %v\n%s", err, out.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("parse capabilities json failed: %v\n%s", err, out.String())
	}
	adapterObj, ok := payload["adapter"].(map[string]any)
	if !ok {
		t.Fatalf("expected adapter object, got %#v", payload)
	}
	if adapterObj["id"] != "codex" {
		t.Fatalf("expected adapter id codex, got %v", adapterObj["id"])
	}
	contractObj, ok := payload["contract"].(map[string]any)
	if !ok {
		t.Fatalf("expected contract object, got %#v", payload)
	}
	if int(contractObj["version"].(float64)) != written.Version {
		t.Fatalf("expected contract version %d, got %v", written.Version, contractObj["version"])
	}
	if contractObj["hash"] != written.Hash {
		t.Fatalf("expected contract hash %s, got %v", written.Hash, contractObj["hash"])
	}
	workflowObj := contractObj["workflow"].(map[string]any)
	if len(workflowObj["phases"].([]any)) == 0 {
		t.Fatalf("expected workflow phases in output, got %#v", workflowObj)
	}
}

func TestCapabilitiesCommandIntegrationContractValidationAndSnapshots(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	t.Setenv("DOCKET_ADAPTER", "")
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	if _, _, err := capabilities.WriteRuntimeContract(tmpRepo, sampleContract()); err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}

	var mdOut bytes.Buffer
	rootCmd.SetOut(&mdOut)
	rootCmd.SetErr(&mdOut)
	rootCmd.SetArgs([]string{"capabilities"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("capabilities markdown failed: %v\n%s", err, mdOut.String())
	}
	md := mdOut.String()
	if !strings.Contains(md, "# Docket Capabilities") || !strings.Contains(md, "## Workflow Phases") {
		t.Fatalf("unexpected markdown output: %s", md)
	}

	var jsonOut bytes.Buffer
	rootCmd.SetOut(&jsonOut)
	rootCmd.SetErr(&jsonOut)
	rootCmd.SetArgs([]string{"capabilities", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("capabilities json failed: %v\n%s", err, jsonOut.String())
	}
	var payload struct {
		Contract capabilities.RuntimeContract `json:"contract"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("parse capabilities json failed: %v\n%s", err, jsonOut.String())
	}

	contract := capabilities.Contract{
		Version:       payload.Contract.Version,
		Workflow:      payload.Contract.Workflow,
		Hooks:         payload.Contract.Hooks,
		Skills:        payload.Contract.Skills,
		Compatibility: payload.Contract.Compatibility,
	}
	if err := capabilities.ValidateContract(contract); err != nil {
		t.Fatalf("expected runtime contract output to validate: %v", err)
	}
	hash, err := capabilities.HashContract(contract)
	if err != nil {
		t.Fatalf("hash contract failed: %v", err)
	}
	if hash != payload.Contract.Hash {
		t.Fatalf("expected output contract hash %s to match recomputed %s", payload.Contract.Hash, hash)
	}

	mdPath := filepath.Join(tmpRepo, "capabilities.md")
	jsonPath := filepath.Join(tmpRepo, "capabilities.json")
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		t.Fatalf("write md snapshot failed: %v", err)
	}
	if err := os.WriteFile(jsonPath, jsonOut.Bytes(), 0o644); err != nil {
		t.Fatalf("write json snapshot failed: %v", err)
	}
	t.Logf("capabilities snapshots: %s %s", mdPath, jsonPath)
}
