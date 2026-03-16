package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/capabilities"
)

const (
	PackVersionV1 = "docket.skills/v1"

	CodeRequired  = "required"
	CodeDuplicate = "duplicate"
)

type SkillMeta struct {
	ID          string `json:"id"`
	Optional    bool   `json:"optional"`
	Instruction string `json:"instruction"`
}

type Pack struct {
	Version      string      `json:"version"`
	ContractHash string      `json:"contract_hash"`
	Skills       []SkillMeta `json:"skills"`
}

type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationReport struct {
	Errors []ValidationError `json:"errors,omitempty"`
}

func (r ValidationReport) Valid() bool {
	return len(r.Errors) == 0
}

type RenderedArtifact struct {
	AdapterID string   `json:"adapter_id"`
	Content   string   `json:"content"`
	SkillIDs  []string `json:"skill_ids"`
	Checksum  string   `json:"checksum"`
}

type MappingReport struct {
	ContractSkillIDs []string `json:"contract_skill_ids"`
	RenderedSkillIDs []string `json:"rendered_skill_ids"`
	Missing          []string `json:"missing,omitempty"`
	Extra            []string `json:"extra,omitempty"`
	InSync           bool     `json:"in_sync"`
	Checksum         string   `json:"checksum"`
}

type validator struct {
	errors []ValidationError
}

func (v *validator) add(path, code, message string) {
	v.errors = append(v.errors, ValidationError{Path: path, Code: code, Message: message})
}

func (v *validator) sortedErrors() []ValidationError {
	sort.SliceStable(v.errors, func(i, j int) bool {
		if v.errors[i].Path == v.errors[j].Path {
			return v.errors[i].Code < v.errors[j].Code
		}
		return v.errors[i].Path < v.errors[j].Path
	})
	return v.errors
}

func BuildPack(contract capabilities.RuntimeContract) (Pack, ValidationReport) {
	v := &validator{}
	pack := Pack{
		Version:      PackVersionV1,
		ContractHash: strings.TrimSpace(contract.Hash),
		Skills:       make([]SkillMeta, 0, len(contract.Skills.Inventory)),
	}
	seen := map[string]int{}
	for i, skill := range contract.Skills.Inventory {
		id := strings.TrimSpace(skill.Name)
		path := fmt.Sprintf("skills.inventory[%d].name", i)
		if id == "" {
			v.add(path, CodeRequired, "skill name is required")
			continue
		}
		key := strings.ToLower(id)
		if prev, ok := seen[key]; ok {
			v.add(path, CodeDuplicate, fmt.Sprintf("duplicate skill name also appears at skills.inventory[%d].name", prev))
			continue
		}
		seen[key] = i
		pack.Skills = append(pack.Skills, SkillMeta{
			ID:          id,
			Optional:    skill.Optional,
			Instruction: fmt.Sprintf("Use `%s` when the task matches this capability.", id),
		})
	}
	return pack, ValidationReport{Errors: v.sortedErrors()}
}

func ContractSkillIDs(contract capabilities.RuntimeContract) []string {
	ids := make([]string, 0, len(contract.Skills.Inventory))
	for _, s := range contract.Skills.Inventory {
		id := strings.TrimSpace(s.Name)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func Render(adapterID string, pack Pack) (RenderedArtifact, error) {
	id := strings.TrimSpace(adapterID)
	if id == "" {
		return RenderedArtifact{}, fmt.Errorf("adapter id is required")
	}
	header := ""
	switch id {
	case "codex":
		header = "# Docket Skill Pack (Codex)"
	case "claude-code":
		header = "## Docket Skill Pack (Claude Code)"
	case "gemini":
		header = "# Docket Skill Pack (Gemini)"
	default:
		return RenderedArtifact{}, fmt.Errorf("unsupported adapter %q", id)
	}

	skillIDs := make([]string, 0, len(pack.Skills))
	lines := []string{
		header,
		"",
		fmt.Sprintf("<!-- docket.skill.pack.version: %s -->", pack.Version),
		fmt.Sprintf("<!-- docket.contract.hash: %s -->", pack.ContractHash),
	}
	for _, s := range pack.Skills {
		skillIDs = append(skillIDs, s.ID)
	}
	lines = append(lines, fmt.Sprintf("<!-- docket.skill.ids: %s -->", strings.Join(skillIDs, ",")))
	lines = append(lines, "", "Use `docket start` to pick up prioritized ticket work.", "", "### Skills")
	for _, s := range pack.Skills {
		kind := "required"
		if s.Optional {
			kind = "optional"
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s): %s", s.ID, kind, s.Instruction))
	}
	content := strings.Join(lines, "\n") + "\n"
	return RenderedArtifact{
		AdapterID: id,
		Content:   content,
		SkillIDs:  skillIDs,
		Checksum:  IDsChecksum(skillIDs),
	}, nil
}

func ExtractSkillIDs(content string) []string {
	const marker = "<!-- docket.skill.ids:"
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		body := strings.TrimPrefix(line, marker)
		body = strings.TrimSuffix(strings.TrimSpace(body), "-->")
		body = strings.TrimSpace(body)
		if body == "" {
			return nil
		}
		parts := strings.Split(body, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			item := strings.TrimSpace(part)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	}
	return nil
}

func BuildMappingReport(contractIDs, renderedIDs []string) MappingReport {
	expected := normalizeIDs(contractIDs)
	actual := normalizeIDs(renderedIDs)
	report := MappingReport{
		ContractSkillIDs: expected,
		RenderedSkillIDs: actual,
		Checksum:         IDsChecksum(actual),
	}
	expectedSet := make(map[string]struct{}, len(expected))
	actualSet := make(map[string]struct{}, len(actual))
	for _, id := range expected {
		expectedSet[id] = struct{}{}
	}
	for _, id := range actual {
		actualSet[id] = struct{}{}
	}
	for _, id := range expected {
		if _, ok := actualSet[id]; !ok {
			report.Missing = append(report.Missing, id)
		}
	}
	for _, id := range actual {
		if _, ok := expectedSet[id]; !ok {
			report.Extra = append(report.Extra, id)
		}
	}
	report.InSync = len(report.Missing) == 0 && len(report.Extra) == 0 && slicesEqual(expected, actual)
	return report
}

func IDsChecksum(ids []string) string {
	sum := sha256.Sum256([]byte(strings.Join(normalizeIDs(ids), ",")))
	return hex.EncodeToString(sum[:])
}

func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
