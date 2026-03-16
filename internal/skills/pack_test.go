package skills

import (
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/capabilities"
)

func TestBuildPackValidationAndRendererConsistency(t *testing.T) {
	contract := capabilities.RuntimeContract{
		Version: 1,
		Hash:    "hash-123",
		Skills: capabilities.SkillInventory{
			Inventory: []capabilities.Skill{
				{Name: "skill-installer", Optional: true},
				{Name: "skill-creator", Optional: false},
			},
		},
	}

	pack, report := BuildPack(contract)
	if !report.Valid() {
		t.Fatalf("expected valid pack report, got %#v", report.Errors)
	}
	if len(pack.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(pack.Skills))
	}

	contractIDs := ContractSkillIDs(contract)
	adapters := []string{"codex", "claude-code", "gemini"}
	for _, adapter := range adapters {
		rendered, err := Render(adapter, pack)
		if err != nil {
			t.Fatalf("render %s failed: %v", adapter, err)
		}
		if !strings.Contains(rendered.Content, "docket.skill.ids:") {
			t.Fatalf("%s output missing machine-readable skill ids marker", adapter)
		}
		gotIDs := ExtractSkillIDs(rendered.Content)
		mapping := BuildMappingReport(contractIDs, gotIDs)
		if !mapping.InSync {
			t.Fatalf("%s mapping should be in sync, got %+v", adapter, mapping)
		}
		if mapping.Checksum == "" {
			t.Fatalf("%s mapping checksum should be populated", adapter)
		}
	}
}

func TestBuildPackRejectsInvalidMetadata(t *testing.T) {
	contract := capabilities.RuntimeContract{
		Version: 1,
		Hash:    "hash-duplicate",
		Skills: capabilities.SkillInventory{
			Inventory: []capabilities.Skill{
				{Name: "skill-installer", Optional: true},
				{Name: "skill-installer", Optional: true},
				{Name: "", Optional: false},
			},
		},
	}

	_, report := BuildPack(contract)
	if report.Valid() {
		t.Fatal("expected invalid metadata report")
	}
	assertHasError(t, report, "skills.inventory[1].name", CodeDuplicate)
	assertHasError(t, report, "skills.inventory[2].name", CodeRequired)
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
