package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type codexAdapter struct{}

func newCodexAdapter() Adapter {
	return codexAdapter{}
}

func (codexAdapter) Metadata() Metadata {
	return Metadata{
		ID:          "codex",
		DisplayName: "Codex",
		Surfaces:    []string{"bootstrap", "doctor", "status", "install"},
	}
}

func (codexAdapter) Detect(repoRoot string) bool {
	return fileExists(filepath.Join(repoRoot, "AGENTS.md"))
}

func (a codexAdapter) Bootstrap(ctx context.Context, input BootstrapInput) error {
	return a.Install(ctx, InstallInput{RepoRoot: input.RepoRoot})
}

func (codexAdapter) Install(_ context.Context, input InstallInput) error {
	repoRoot := strings.TrimSpace(input.RepoRoot)
	if repoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	if err := ensureCodexAgentsFile(repoRoot); err != nil {
		return err
	}
	if err := ensureCodexConfig(repoRoot); err != nil {
		return err
	}
	return nil
}

func (codexAdapter) Doctor(_ context.Context, repoRoot string) (DoctorReport, error) {
	checks := []DoctorCheck{
		buildCodexAgentsCheck(repoRoot),
		buildCodexConfigCheck(repoRoot),
		buildCodexHooksCheck(repoRoot),
	}
	return DoctorReport{Checks: checks}, nil
}

func (a codexAdapter) Status(ctx context.Context, repoRoot string) (StatusReport, error) {
	report, err := a.Doctor(ctx, repoRoot)
	if err != nil {
		return StatusReport{}, err
	}
	ready := true
	for _, chk := range report.Checks {
		if !chk.OK {
			ready = false
			break
		}
	}
	if ready {
		return StatusReport{Ready: true, Summary: "codex adapter ready"}, nil
	}
	return StatusReport{Ready: false, Summary: "run `docket bootstrap --adapter codex`"}, nil
}

func codexConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, "doombox.json")
}

func ensureCodexAgentsFile(repoRoot string) error {
	path := filepath.Join(repoRoot, "AGENTS.md")
	block, err := renderSkillPackBlock(repoRoot, "codex")
	if err != nil {
		return err
	}
	return upsertManagedSkillBlock(path, block)
}

func ensureCodexConfig(repoRoot string) error {
	path := codexConfigPath(repoRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		payload := map[string]any{"mcp": "docket"}
		return writeJSONFile(path, payload)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if strings.EqualFold(fmt.Sprint(payload["mcp"]), "docket") {
		return nil
	}
	payload["mcp"] = "docket"
	return writeJSONFile(path, payload)
}

func writeJSONFile(path string, payload map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func buildCodexAgentsCheck(repoRoot string) DoctorCheck {
	path := filepath.Join(repoRoot, "AGENTS.md")
	if fileExists(path) {
		return DoctorCheck{Name: "codex_agents", OK: true, Detail: "AGENTS.md detected."}
	}
	return DoctorCheck{Name: "codex_agents", OK: false, Detail: "AGENTS.md missing. Run `docket bootstrap --adapter codex`."}
}

func buildCodexConfigCheck(repoRoot string) DoctorCheck {
	path := codexConfigPath(repoRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		return DoctorCheck{Name: "codex_config", OK: false, Detail: "doombox.json missing. Run `docket bootstrap --adapter codex`."}
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return DoctorCheck{Name: "codex_config", OK: false, Detail: "doombox.json is invalid JSON."}
	}
	if strings.EqualFold(fmt.Sprint(payload["mcp"]), "docket") {
		return DoctorCheck{Name: "codex_config", OK: true, Detail: "doombox.json mcp wiring is configured."}
	}
	return DoctorCheck{Name: "codex_config", OK: false, Detail: "doombox.json missing mcp=docket. Run `docket bootstrap --adapter codex`."}
}

func buildCodexHooksCheck(repoRoot string) DoctorCheck {
	paths := []string{
		filepath.Join(repoRoot, ".git", "hooks", "pre-commit"),
		filepath.Join(repoRoot, ".git", "hooks", "commit-msg"),
		filepath.Join(repoRoot, ".git", "hooks", "post-merge"),
	}
	for _, path := range paths {
		if !fileExists(path) {
			return DoctorCheck{Name: "hooks", OK: false, Detail: "Git hooks missing. Run `docket bootstrap --adapter codex`."}
		}
	}
	return DoctorCheck{Name: "hooks", OK: true, Detail: "Git hooks detected."}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
