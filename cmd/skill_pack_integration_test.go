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
		readPaths func(repoRoot string) ([]string, error)
	}{
		{
			adapterID: "codex",
			readPaths: func(repoRoot string) ([]string, error) {
				home, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				return []string{
					filepath.Join(repoRoot, "AGENTS.md"),
					filepath.Join(home, ".codex", "skills", "docket", "SKILL.md"),
				}, nil
			},
		},
		{
			adapterID: "claude-code",
			readPaths: func(repoRoot string) ([]string, error) {
				return []string{filepath.Join(repoRoot, "CLAUDE.md")}, nil
			},
		},
		{
			adapterID: "gemini",
			readPaths: func(repoRoot string) ([]string, error) {
				home, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				return []string{filepath.Join(home, ".gemini", "skills", "docket", "SKILL.md")}, nil
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

			artifactPaths, err := fixture.readPaths(h.repo)
			if err != nil {
				t.Fatalf("resolve artifact paths failed: %v", err)
			}
			pack, report := skills.BuildPack(runtime)
			if !report.Valid() {
				t.Fatalf("expected canonical skill metadata to build pack, got %+v", report.Errors)
			}

			for idx, artifactPath := range artifactPaths {
				content, err := os.ReadFile(artifactPath)
				if err != nil {
					t.Fatalf("read skill artifact failed (%s): %v", artifactPath, err)
				}
				actualIDs := skills.ExtractSkillIDs(string(content))
				mapping := skills.BuildMappingReport(expectedIDs, actualIDs)
				if !mapping.InSync {
					t.Fatalf("expected contract skill IDs to match installed artifact for %s (%s), got %+v", fixture.adapterID, artifactPath, mapping)
				}
				gotMetadataChecksum := skills.ExtractSkillMetadataChecksum(string(content))
				if gotMetadataChecksum != pack.MetadataChecksum {
					t.Fatalf("expected metadata checksum %s for %s artifact %s, got %s", pack.MetadataChecksum, fixture.adapterID, artifactPath, gotMetadataChecksum)
				}

				reportData, err := json.MarshalIndent(mapping, "", "  ")
				if err != nil {
					t.Fatalf("marshal mapping report failed: %v", err)
				}
				mapFixture := h.writeFixture(filepath.Join("skills", fixture.adapterID, "mapping-report-"+filepath.Base(artifactPath)+".json"), append(reportData, '\n'))
				checksumFixture := h.writeFixture(filepath.Join("skills", fixture.adapterID, "checksum-"+filepath.Base(artifactPath)+".txt"), []byte(mapping.Checksum+"\n"))
				t.Logf("skill mapping fixtures [%d]: %s | %s", idx, mapFixture, checksumFixture)
			}
		})
	}
}
