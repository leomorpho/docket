package capabilities

import "path/filepath"

func CanonicalContractV1() Contract {
	return Contract{
		Version: ContractVersion,
		Workflow: WorkflowCapabilities{
			Phases: []string{"bootstrap", "start", "plan", "implement", "verify"},
		},
		Hooks: HookCapabilities{
			Events: []HookEvent{
				{Name: "run.start", Blocking: true},
				{Name: "ticket.review", Blocking: true},
				{Name: "ticket.qa", Blocking: false},
				{Name: "ticket.privileged", Blocking: true},
			},
		},
		Skills: SkillInventory{
			Inventory: []Skill{
				{Name: "skill-installer", Optional: true},
				{Name: "skill-creator", Optional: true},
			},
		},
		Compatibility: CompatibilityNotes{
			BackwardCompatibleWith: []int{ContractVersion},
			UpgradeNotes:           "Regenerate adapter skill packs when the canonical contract hash changes.",
		},
	}
}

func EnsureRuntimeContract(repoRoot string) (RuntimeContract, string, error) {
	runtime, err := LoadRuntimeContract(repoRoot)
	if err == nil {
		return runtime, filepath.Join(repoRoot, DefaultRuntimeContractPath), nil
	}
	return WriteRuntimeContract(repoRoot, CanonicalContractV1())
}
