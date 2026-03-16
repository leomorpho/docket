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

func TestDoctorCheckTransitionsAndRemediationFormatting(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "AGENTS.md"), []byte("codex"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "doombox.json"), []byte(`{"mcp":"docket"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}

	before := buildDoctorReport(repoRoot)
	if statusByName(before.Checks, "mcp") != "PASS" {
		t.Fatalf("expected mcp PASS before hook setup")
	}
	if statusByName(before.Checks, "skills") != "PASS" {
		t.Fatalf("expected skills PASS before hook setup")
	}
	if statusByName(before.Checks, "hooks") != "FAIL" {
		t.Fatalf("expected hooks FAIL before hook setup")
	}
	if statusByName(before.Checks, "contract") != "FAIL" {
		t.Fatalf("expected contract FAIL before contract write")
	}
	for _, chk := range before.Checks {
		if chk.Status == "FAIL" && !strings.Contains(chk.Remediation, "docket") {
			t.Fatalf("expected actionable remediation command for %s, got %q", chk.Name, chk.Remediation)
		}
	}

	if _, err := writeHook(repoRoot); err != nil {
		t.Fatalf("writeHook failed: %v", err)
	}
	if _, _, err := capabilities.WriteRuntimeContract(repoRoot, sampleContract()); err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}

	after := buildDoctorReport(repoRoot)
	if statusByName(after.Checks, "hooks") != "PASS" {
		t.Fatalf("expected hooks PASS after hook setup")
	}
	if statusByName(after.Checks, "contract") != "PASS" {
		t.Fatalf("expected contract PASS after contract write")
	}
}

func TestDoctorBeforeAfterBootstrapJSONArtifacts(t *testing.T) {
	tmpRepo := t.TempDir()
	tmpHome := filepath.Join(t.TempDir(), "docket-home")
	t.Setenv("DOCKET_HOME", tmpHome)
	docketHome = ""
	repo = tmpRepo
	format = "human"

	if err := os.MkdirAll(filepath.Join(tmpRepo, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "AGENTS.md"), []byte("codex marker"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpRepo, "doombox.json"), []byte(`{"mcp":"docket"}`), 0o644); err != nil {
		t.Fatalf("write doombox.json failed: %v", err)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"doctor", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("doctor before bootstrap failed: %v", err)
	}
	beforeJSON := out.Bytes()
	var before doctorReport
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatalf("unmarshal before report failed: %v\n%s", err, string(beforeJSON))
	}
	if statusByName(before.Checks, "hooks") != "FAIL" {
		t.Fatalf("expected hooks FAIL before bootstrap")
	}

	out.Reset()
	rootCmd.SetArgs([]string{"bootstrap", "--adapter", "codex"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	out.Reset()
	rootCmd.SetArgs([]string{"doctor", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("doctor after bootstrap failed: %v", err)
	}
	afterJSON := out.Bytes()
	var after doctorReport
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("unmarshal after report failed: %v\n%s", err, string(afterJSON))
	}
	if statusByName(after.Checks, "hooks") != "PASS" {
		t.Fatalf("expected hooks PASS after bootstrap")
	}

	beforePath := filepath.Join(tmpRepo, "doctor-before.json")
	afterPath := filepath.Join(tmpRepo, "doctor-after.json")
	if err := os.WriteFile(beforePath, beforeJSON, 0o644); err != nil {
		t.Fatalf("write before artifact failed: %v", err)
	}
	if err := os.WriteFile(afterPath, afterJSON, 0o644); err != nil {
		t.Fatalf("write after artifact failed: %v", err)
	}
	t.Logf("doctor json artifacts: before=%s after=%s", beforePath, afterPath)
}

func statusByName(checks []doctorCheck, name string) string {
	for _, c := range checks {
		if c.Name == name {
			return c.Status
		}
	}
	return ""
}

func sampleContract() capabilities.Contract {
	return capabilities.Contract{
		Version: capabilities.ContractVersion,
		Workflow: capabilities.WorkflowCapabilities{
			Phases: []string{"bootstrap", "start", "plan", "implement", "verify"},
		},
		Hooks: capabilities.HookCapabilities{
			Events: []capabilities.HookEvent{
				{Name: "run_start", Blocking: true},
				{Name: "state_transition", Blocking: false},
			},
		},
		Skills: capabilities.SkillInventory{
			Inventory: []capabilities.Skill{
				{Name: "skill-installer", Optional: true},
			},
		},
		Compatibility: capabilities.CompatibilityNotes{
			BackwardCompatibleWith: []int{1},
			UpgradeNotes:           "Preserve unknown fields when parsing future versions.",
		},
	}
}
