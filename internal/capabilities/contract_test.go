package capabilities

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/artifacts"
)

func TestValidateContractRequiresCoreFields(t *testing.T) {
	base := validContract()
	cases := []struct {
		name string
		mut  func(*Contract)
	}{
		{
			name: "missing workflow phases",
			mut: func(c *Contract) {
				c.Workflow.Phases = nil
			},
		},
		{
			name: "missing hook events",
			mut: func(c *Contract) {
				c.Hooks.Events = nil
			},
		},
		{
			name: "invalid hook namespace",
			mut: func(c *Contract) {
				c.Hooks.Namespace = "hooks"
			},
		},
		{
			name: "invalid hook invocation",
			mut: func(c *Contract) {
				c.Hooks.Invocation = "agent-invoked"
			},
		},
		{
			name: "invalid hook execution mode",
			mut: func(c *Contract) {
				c.Hooks.Execution = "user-invoked"
			},
		},
		{
			name: "hook event without name",
			mut: func(c *Contract) {
				c.Hooks.Events[0].Name = ""
			},
		},
		{
			name: "invalid hook event mode",
			mut: func(c *Contract) {
				c.Hooks.Events[0].Mode = "unknown"
			},
		},
		{
			name: "missing skills",
			mut: func(c *Contract) {
				c.Skills.Inventory = nil
			},
		},
		{
			name: "invalid skills namespace",
			mut: func(c *Contract) {
				c.Skills.Namespace = "hooks"
			},
		},
		{
			name: "invalid skills invocation",
			mut: func(c *Contract) {
				c.Skills.Invocation = "system-run"
			},
		},
		{
			name: "skill without name",
			mut: func(c *Contract) {
				c.Skills.Inventory[0].Name = ""
			},
		},
		{
			name: "missing compatibility note",
			mut: func(c *Contract) {
				c.Compatibility.UpgradeNotes = ""
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			err := ValidateContract(c)
			if !errors.Is(err, ErrInvalidContract) {
				t.Fatalf("expected ErrInvalidContract, got: %v", err)
			}
		})
	}
}

func TestHashContractIsDeterministicForUnchangedPayload(t *testing.T) {
	contract := validContract()

	first, err := HashContract(contract)
	if err != nil {
		t.Fatalf("HashContract(first) failed: %v", err)
	}
	second, err := HashContract(contract)
	if err != nil {
		t.Fatalf("HashContract(second) failed: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable hash, got %q and %q", first, second)
	}
}

func TestWriteAndLoadRoundTrip(t *testing.T) {
	repo := t.TempDir()
	contract := validContract()

	written, path, err := WriteRuntimeContract(repo, contract)
	if err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}
	if path == "" {
		t.Fatalf("expected output path")
	}
	if want := filepath.Join(repo, ".docket", "local", "runtime", "capabilities.json"); path != want {
		t.Fatalf("expected canonical runtime contract path %q, got %q", want, path)
	}
	if written.Hash == "" {
		t.Fatalf("expected runtime hash")
	}

	loaded, err := LoadRuntimeContract(repo)
	if err != nil {
		t.Fatalf("LoadRuntimeContract failed: %v", err)
	}

	if !reflect.DeepEqual(written, loaded) {
		t.Fatalf("roundtrip mismatch\nwritten=%#v\nloaded=%#v", written, loaded)
	}
	t.Logf("capabilities contract version=%d hash=%s", loaded.Version, loaded.Hash)
}

func TestLoadRuntimeContractFallsBackToLegacyPath(t *testing.T) {
	repo := t.TempDir()
	runtime, _, err := WriteRuntimeContract(t.TempDir(), validContract())
	if err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}

	legacyPath := artifacts.LegacyRepoPath(repo, artifacts.RepoRuntimeCapabilities)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy path: %v", err)
	}
	data, err := json.MarshalIndent(runtime, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(legacyPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy path: %v", err)
	}

	loaded, err := LoadRuntimeContract(repo)
	if err != nil {
		t.Fatalf("LoadRuntimeContract failed: %v", err)
	}
	if !reflect.DeepEqual(runtime, loaded) {
		t.Fatalf("legacy fallback mismatch\nwant=%#v\ngot=%#v", runtime, loaded)
	}
}

func TestLoadRuntimeContractDetectsHashMismatch(t *testing.T) {
	repo := t.TempDir()
	written, path, err := WriteRuntimeContract(repo, validContract())
	if err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}

	written.Hash = "deadbeef"
	data, err := json.MarshalIndent(written, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("Write tampered contract failed: %v", err)
	}

	_, err = LoadRuntimeContract(repo)
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got: %v", err)
	}
}

func TestHashContractDefaultsVersionWhenUnset(t *testing.T) {
	contract := validContract()
	contract.Version = 0

	hash, err := HashContract(contract)
	if err != nil {
		t.Fatalf("HashContract failed: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}

	runtime, _, err := WriteRuntimeContract(t.TempDir(), contract)
	if err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}
	if runtime.Version != ContractVersion {
		t.Fatalf("expected default contract version %d, got %d", ContractVersion, runtime.Version)
	}
}

func TestValidateContractBackfillsLegacyNamespaceDefaults(t *testing.T) {
	contract := validContract()
	contract.Hooks.Namespace = ""
	contract.Hooks.Invocation = ""
	contract.Hooks.Execution = ""
	contract.Hooks.Events[0].Mode = ""
	contract.Hooks.Events[0].Blocking = true
	contract.Hooks.Events[1].Mode = ""
	contract.Hooks.Events[1].Blocking = false
	contract.Skills.Namespace = ""
	contract.Skills.Invocation = ""
	contract.Skills.Inventory[0].Title = ""
	contract.Skills.Inventory[0].Summary = ""
	contract.Skills.Inventory[0].Intent = ""
	contract.Skills.Inventory[0].Command = ""
	contract.Skills.Inventory[0].Triggers = nil

	if err := ValidateContract(contract); err != nil {
		t.Fatalf("expected legacy defaults to validate, got: %v", err)
	}
}

func TestCanonicalContractContextOptimizeCommandMatchesCLI(t *testing.T) {
	contract := CanonicalContractV1()

	var contextOptimize *Skill
	for i := range contract.Skills.Inventory {
		if contract.Skills.Inventory[i].Name == "context-optimize" {
			contextOptimize = &contract.Skills.Inventory[i]
			break
		}
	}
	if contextOptimize == nil {
		t.Fatal("expected context-optimize skill in canonical contract")
	}
	if got, want := contextOptimize.Command, "docket context-optimize {ticket_id}"; got != want {
		t.Fatalf("context-optimize command = %q, want %q", got, want)
	}
	if strings.Contains(contextOptimize.Command, "context optimize") {
		t.Fatalf("context-optimize command should use the hyphenated CLI form, got %q", contextOptimize.Command)
	}
}

func TestWriteRuntimeContractBackfillsSkillMetadataDefaults(t *testing.T) {
	contract := validContract()
	contract.Skills.Inventory[0].Title = ""
	contract.Skills.Inventory[0].Summary = ""
	contract.Skills.Inventory[0].Intent = ""
	contract.Skills.Inventory[0].Triggers = nil

	runtime, _, err := WriteRuntimeContract(t.TempDir(), contract)
	if err != nil {
		t.Fatalf("WriteRuntimeContract failed: %v", err)
	}
	got := runtime.Skills.Inventory[0]
	if got.Title == "" || got.Summary == "" || got.Intent == "" || got.Command == "" {
		t.Fatalf("expected skill metadata defaults, got %#v", got)
	}
	if len(got.Triggers) == 0 || got.Triggers[0] != "manual" {
		t.Fatalf("expected default manual trigger, got %#v", got.Triggers)
	}
}

func validContract() Contract {
	return Contract{
		Version: ContractVersion,
		Workflow: WorkflowCapabilities{
			Phases: []string{"plan", "implement", "verify"},
		},
		Hooks: HookCapabilities{
			Namespace:  HookNamespaceSystem,
			Invocation: HookInvocationSystem,
			Execution:  HookExecutionInternal,
			Events: []HookEvent{
				{Name: "run_start", Mode: HookModeEnforcement},
				{Name: "state_transition", Mode: HookModeAdvisory},
			},
		},
		Skills: SkillInventory{
			Namespace:  SkillNamespaceAgent,
			Invocation: SkillInvocationAgent,
			Inventory: []Skill{
				{
					Name:     "ticket-discovery",
					Title:    "Discover Next Ticket",
					Summary:  "Find the next actionable ticket and inspect context.",
					Intent:   "planning",
					Command:  "docket list --state open --format context",
					Triggers: []string{"session_start", "resume"},
					Optional: true,
				},
				{
					Name:     "ticket-authoring-apply",
					Title:    "Transactional Ticket Authoring",
					Summary:  "Use scaffold/apply for robust structured authoring.",
					Intent:   "authoring",
					Command:  "docket ticket scaffold --format json",
					Triggers: []string{"automation_mode"},
					Optional: true,
				},
			},
		},
		Compatibility: CompatibilityNotes{
			BackwardCompatibleWith: []int{1},
			UpgradeNotes:           "Future versions must bump version and preserve unknown fields where possible.",
		},
	}
}
