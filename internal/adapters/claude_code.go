package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type claudeCodeAdapter struct{}

func newClaudeCodeAdapter() Adapter {
	return claudeCodeAdapter{}
}

func (claudeCodeAdapter) Metadata() Metadata {
	return Metadata{
		ID:          "claude-code",
		DisplayName: "Claude Code",
		Surfaces:    []string{"bootstrap", "doctor", "status", "install"},
	}
}

func (claudeCodeAdapter) Detect(repoRoot string) bool {
	return fileExists(filepath.Join(repoRoot, "CLAUDE.md"))
}

func (a claudeCodeAdapter) Bootstrap(ctx context.Context, input BootstrapInput) error {
	return a.Install(ctx, InstallInput{RepoRoot: input.RepoRoot})
}

func (claudeCodeAdapter) Install(_ context.Context, input InstallInput) error {
	repoRoot := strings.TrimSpace(input.RepoRoot)
	if repoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	if err := ensureClaudeDoc(repoRoot); err != nil {
		return err
	}
	if err := ensureClaudeMCPConfig(repoRoot); err != nil {
		return err
	}
	return nil
}

func (claudeCodeAdapter) Doctor(_ context.Context, repoRoot string) (DoctorReport, error) {
	checks := []DoctorCheck{
		buildClaudeDocCheck(repoRoot),
		buildClaudeMCPCheck(repoRoot),
		buildCodexHooksCheck(repoRoot),
	}
	return DoctorReport{Checks: checks}, nil
}

func (a claudeCodeAdapter) Status(ctx context.Context, repoRoot string) (StatusReport, error) {
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
		return StatusReport{Ready: true, Summary: "claude-code adapter ready"}, nil
	}
	return StatusReport{Ready: false, Summary: "run `docket bootstrap --adapter claude-code`"}, nil
}

func ensureClaudeDoc(repoRoot string) error {
	path := filepath.Join(repoRoot, "CLAUDE.md")
	block, err := renderSkillPackBlock(repoRoot, "claude-code")
	if err != nil {
		return err
	}
	return upsertManagedSkillBlock(path, block)
}

func ensureClaudeMCPConfig(repoRoot string) error {
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

func buildClaudeDocCheck(repoRoot string) DoctorCheck {
	path := filepath.Join(repoRoot, "CLAUDE.md")
	if fileExists(path) {
		return DoctorCheck{Name: "claude_doc", OK: true, Detail: "CLAUDE.md detected."}
	}
	return DoctorCheck{Name: "claude_doc", OK: false, Detail: "CLAUDE.md missing. Run `docket bootstrap --adapter claude-code`."}
}

func buildClaudeMCPCheck(repoRoot string) DoctorCheck {
	path := filepath.Join(repoRoot, ".cursor", "mcp.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return DoctorCheck{Name: "claude_mcp", OK: false, Detail: "Claude MCP config missing. Run `docket bootstrap --adapter claude-code`."}
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return DoctorCheck{Name: "claude_mcp", OK: false, Detail: "Claude MCP config is invalid JSON."}
	}
	servers, _ := payload["servers"].(map[string]any)
	if servers == nil {
		return DoctorCheck{Name: "claude_mcp", OK: false, Detail: "Claude MCP config missing servers object."}
	}
	if _, ok := servers["docket"]; ok {
		return DoctorCheck{Name: "claude_mcp", OK: true, Detail: "Claude MCP docket server is configured."}
	}
	return DoctorCheck{Name: "claude_mcp", OK: false, Detail: "Claude MCP docket server is missing. Run `docket bootstrap --adapter claude-code`."}
}
