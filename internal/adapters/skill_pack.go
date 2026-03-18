package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/skills"
)

const (
	skillPackStartMarker = "<!-- docket:skill-pack:start -->"
	skillPackEndMarker   = "<!-- docket:skill-pack:end -->"
)

func renderSkillPackBlock(repoRoot, adapterID string) (string, error) {
	runtime, _, err := capabilities.EnsureRuntimeContract(repoRoot)
	if err != nil {
		return "", err
	}
	pack, report := skills.BuildPack(runtime)
	if !report.Valid() {
		return "", fmt.Errorf("invalid skill metadata: %#v", report.Errors)
	}
	rendered, err := skills.Render(adapterID, pack)
	if err != nil {
		return "", err
	}
	mapping := skills.BuildMappingReport(skills.ContractSkillIDs(runtime), rendered.SkillIDs)
	if !mapping.InSync {
		return "", fmt.Errorf("skill mapping drift detected (missing=%v extra=%v)", mapping.Missing, mapping.Extra)
	}
	if rendered.MetadataChecksum != pack.MetadataChecksum {
		return "", fmt.Errorf("skill metadata drift detected (expected=%s actual=%s)", pack.MetadataChecksum, rendered.MetadataChecksum)
	}
	return skillPackStartMarker + "\n" + rendered.Content + skillPackEndMarker + "\n", nil
}

func upsertManagedSkillBlock(path, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(block), 0o644)
		}
		return err
	}
	text := string(raw)
	start := strings.Index(text, skillPackStartMarker)
	end := strings.Index(text, skillPackEndMarker)
	if start >= 0 && end >= start {
		end += len(skillPackEndMarker)
		updated := text[:start] + block + text[end:]
		if !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		return os.WriteFile(path, []byte(updated), 0o644)
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n" + block
	return os.WriteFile(path, []byte(text), 0o644)
}
