package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/leomorpho/docket/internal/skills"
)

func TestBootstrapAdapterSkillIDsAlignWithCanonicalContract(t *testing.T) {
	fixtures := []struct {
		adapterID string
		readPath  func(repoRoot string) (string, error)
	}{
		{
			adapterID: "codex",
			readPath: func(repoRoot string) (string, error) {
				return filepath.Join(repoRoot, "AGENTS.md"), nil
			},
		},
		{
			adapterID: "claude-code",
			readPath: func(repoRoot string) (string, error) {
				return filepath.Join(repoRoot, "CLAUDE.md"), nil
			},
		},
		{
			adapterID: "gemini",
			readPath: func(repoRoot string) (string, error) {
				home, err := os.UserHomeDir()
				if err != nil {
					return "", err
				}
				return filepath.Join(home, ".gemini", "skills", "docket", "SKILL.md"), nil
			},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.adapterID, func(t *testing.T) {
			h := newFakeRepoHarnessForAdapter(t, fixture.adapterID)
			if out, err := h.run("bootstrap", "--adapter", fixture.adapterID); err != nil {
				t.Fatalf("bootstrap failed: %v\n%s", err, out)
			}

			runtime, err := capabilities.LoadRuntimeContract(h.repo)
			if err != nil {
				t.Fatalf("expected runtime contract after bootstrap: %v", err)
			}
			expectedIDs := skills.ContractSkillIDs(runtime)

			artifactPath, err := fixture.readPath(h.repo)
			if err != nil {
				t.Fatalf("resolve artifact path failed: %v", err)
			}
			content, err := os.ReadFile(artifactPath)
			if err != nil {
				t.Fatalf("read skill artifact failed: %v", err)
			}
			actualIDs := skills.ExtractSkillIDs(string(content))
			mapping := skills.BuildMappingReport(expectedIDs, actualIDs)
			if !mapping.InSync {
				t.Fatalf("expected contract skill IDs to match installed artifact for %s, got %+v", fixture.adapterID, mapping)
			}

			reportData, err := json.MarshalIndent(mapping, "", "  ")
			if err != nil {
				t.Fatalf("marshal mapping report failed: %v", err)
			}
			mapFixture := h.writeFixture(filepath.Join("skills", fixture.adapterID, "mapping-report.json"), append(reportData, '\n'))
			checksumFixture := h.writeFixture(filepath.Join("skills", fixture.adapterID, "checksum.txt"), []byte(mapping.Checksum+"\n"))
			t.Logf("skill mapping fixtures: %s | %s", mapFixture, checksumFixture)
		})
	}
}
