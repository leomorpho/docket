package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const geminiManagedBlock = "# Docket\nUse `docket start` to work the next prioritized ticket."

type geminiAdapter struct{}

func newGeminiAdapter() Adapter {
	return geminiAdapter{}
}

func (geminiAdapter) Metadata() Metadata {
	return Metadata{
		ID:          "gemini",
		DisplayName: "Gemini CLI",
		Surfaces:    []string{"bootstrap", "doctor", "status", "install"},
	}
}

func (geminiAdapter) Detect(repoRoot string) bool {
	return fileExists(filepath.Join(repoRoot, "GEMINI.md"))
}

func (a geminiAdapter) Bootstrap(ctx context.Context, input BootstrapInput) error {
	return a.Install(ctx, InstallInput{RepoRoot: input.RepoRoot})
}

func (geminiAdapter) Install(_ context.Context, input InstallInput) error {
	repoRoot := strings.TrimSpace(input.RepoRoot)
	if repoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	if err := ensureGeminiDoc(repoRoot); err != nil {
		return err
	}
	if err := ensureGeminiSkill(); err != nil {
		return err
	}
	if err := ensureGeminiSettings(); err != nil {
		return err
	}
	if err := ensureGeminiRepoMCP(repoRoot); err != nil {
		return err
	}
	return nil
}

func (geminiAdapter) Doctor(_ context.Context, repoRoot string) (DoctorReport, error) {
	checks := []DoctorCheck{
		buildGeminiDocCheck(repoRoot),
		buildGeminiSkillCheck(),
		buildGeminiSettingsCheck(),
		buildGeminiRepoMCPCheck(repoRoot),
		buildCodexHooksCheck(repoRoot),
	}
	return DoctorReport{Checks: checks}, nil
}

func (a geminiAdapter) Status(ctx context.Context, repoRoot string) (StatusReport, error) {
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
		return StatusReport{Ready: true, Summary: "gemini adapter ready"}, nil
	}
	return StatusReport{Ready: false, Summary: "run `docket bootstrap --adapter gemini`"}, nil
}

func ensureGeminiDoc(repoRoot string) error {
	path := filepath.Join(repoRoot, "GEMINI.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.WriteFile(path, []byte(geminiManagedBlock+"\n"), 0o644)
	}
	text := string(raw)
	if strings.Contains(strings.ToLower(text), "docket") {
		return nil
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n" + geminiManagedBlock + "\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

func geminiSkillPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "skills", "docket", "SKILL.md"), nil
}

func ensureGeminiSkill() error {
	path, err := geminiSkillPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if fileExists(path) {
		return nil
	}
	return os.WriteFile(path, []byte("# Docket Skill\nUse docket commands for deterministic ticket workflows.\n"), 0o644)
}

func geminiSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini", "settings.json"), nil
}

func ensureGeminiSettings() error {
	path, err := geminiSettingsPath()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		payload := map[string]any{
			"mcp": map[string]any{
				"docket": map[string]any{"command": "docket", "args": []string{"serve", "--mcp"}},
			},
		}
		return writeJSONFile(path, payload)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	mcp, _ := payload["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}
	if _, ok := mcp["docket"]; ok {
		payload["mcp"] = mcp
		return nil
	}
	mcp["docket"] = map[string]any{"command": "docket", "args": []string{"serve", "--mcp"}}
	payload["mcp"] = mcp
	return writeJSONFile(path, payload)
}

func ensureGeminiRepoMCP(repoRoot string) error {
	path := filepath.Join(repoRoot, ".cursor", "mcp.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		payload := map[string]any{
			"servers": map[string]any{
				"docket": map[string]any{"command": "docket", "args": []string{"serve", "--mcp"}},
			},
		}
		return writeJSONFile(path, payload)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	servers, _ := payload["servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if _, ok := servers["docket"]; ok {
		payload["servers"] = servers
		return nil
	}
	servers["docket"] = map[string]any{"command": "docket", "args": []string{"serve", "--mcp"}}
	payload["servers"] = servers
	return writeJSONFile(path, payload)
}

func buildGeminiDocCheck(repoRoot string) DoctorCheck {
	if fileExists(filepath.Join(repoRoot, "GEMINI.md")) {
		return DoctorCheck{Name: "gemini_doc", OK: true, Detail: "GEMINI.md detected."}
	}
	return DoctorCheck{Name: "gemini_doc", OK: false, Detail: "GEMINI.md missing. Run `docket bootstrap --adapter gemini`."}
}

func buildGeminiSkillCheck() DoctorCheck {
	path, err := geminiSkillPath()
	if err == nil && fileExists(path) {
		return DoctorCheck{Name: "gemini_skill", OK: true, Detail: "Gemini docket skill is installed."}
	}
	return DoctorCheck{Name: "gemini_skill", OK: false, Detail: "Gemini skill missing. Run `docket bootstrap --adapter gemini`."}
}

func buildGeminiSettingsCheck() DoctorCheck {
	path, err := geminiSettingsPath()
	if err != nil {
		return DoctorCheck{Name: "gemini_settings", OK: false, Detail: "Gemini settings path unavailable."}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return DoctorCheck{Name: "gemini_settings", OK: false, Detail: "Gemini settings missing. Run `docket bootstrap --adapter gemini`."}
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return DoctorCheck{Name: "gemini_settings", OK: false, Detail: "Gemini settings JSON is invalid."}
	}
	mcp, _ := payload["mcp"].(map[string]any)
	if mcp != nil {
		if _, ok := mcp["docket"]; ok {
			return DoctorCheck{Name: "gemini_settings", OK: true, Detail: "Gemini settings include docket MCP config."}
		}
	}
	return DoctorCheck{Name: "gemini_settings", OK: false, Detail: "Gemini settings missing docket MCP config. Run `docket bootstrap --adapter gemini`."}
}

func buildGeminiRepoMCPCheck(repoRoot string) DoctorCheck {
	path := filepath.Join(repoRoot, ".cursor", "mcp.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return DoctorCheck{Name: "gemini_repo_mcp", OK: false, Detail: "Repo MCP config missing. Run `docket bootstrap --adapter gemini`."}
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return DoctorCheck{Name: "gemini_repo_mcp", OK: false, Detail: "Repo MCP config JSON is invalid."}
	}
	servers, _ := payload["servers"].(map[string]any)
	if servers != nil {
		if _, ok := servers["docket"]; ok {
			return DoctorCheck{Name: "gemini_repo_mcp", OK: true, Detail: "Repo MCP docket server is configured."}
		}
	}
	return DoctorCheck{Name: "gemini_repo_mcp", OK: false, Detail: "Repo MCP docket server missing. Run `docket bootstrap --adapter gemini`."}
}
