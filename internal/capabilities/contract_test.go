package capabilities

import (
	"errors"
	"reflect"
	"testing"
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
			name: "hook event without name",
			mut: func(c *Contract) {
				c.Hooks.Events[0].Name = ""
			},
		},
		{
			name: "missing skills",
			mut: func(c *Contract) {
				c.Skills.Inventory = nil
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

func validContract() Contract {
	return Contract{
		Version: ContractVersion,
		Workflow: WorkflowCapabilities{
			Phases: []string{"plan", "implement", "verify"},
		},
		Hooks: HookCapabilities{
			Events: []HookEvent{
				{Name: "run_start", Blocking: true},
				{Name: "state_transition", Blocking: false},
			},
		},
		Skills: SkillInventory{
			Inventory: []Skill{
				{Name: "skill-installer", Optional: true},
				{Name: "skill-creator", Optional: true},
			},
		},
		Compatibility: CompatibilityNotes{
			BackwardCompatibleWith: []int{1},
			UpgradeNotes:           "Future versions must bump version and preserve unknown fields where possible.",
		},
	}
}
