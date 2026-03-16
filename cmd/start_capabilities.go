package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/adapters"
)

type startReadiness struct {
	MCP    string `json:"mcp"`
	Skills string `json:"skills"`
	Hooks  string `json:"hooks"`
}

type startCapabilityDigest struct {
	Adapter     string         `json:"adapter"`
	FlowPhases  []string       `json:"flow_phases"`
	Readiness   startReadiness `json:"readiness"`
	Remediation string         `json:"remediation,omitempty"`
}

func buildStartCapabilityDigest(repoRoot string) startCapabilityDigest {
	adapterID := adapters.DefaultRegistry().Resolve(repoRoot, "").Metadata().ID
	if strings.TrimSpace(adapterID) == "" {
		adapterID = "unknown"
	}

	mcpReady := isMCPReady(repoRoot)
	skillsReady := isSkillsReady(repoRoot, adapterID)
	hooksReady := isHooksReady(repoRoot)

	out := startCapabilityDigest{
		Adapter:    adapterID,
		FlowPhases: []string{"bootstrap", "start", "plan", "implement", "verify"},
		Readiness: startReadiness{
			MCP:    readinessLabel(mcpReady),
			Skills: readinessLabel(skillsReady),
			Hooks:  readinessLabel(hooksReady),
		},
	}
	if !(mcpReady && skillsReady && hooksReady) {
		out.Remediation = "Run `docket bootstrap` to install or repair integration artifacts."
	}
	return out
}

func renderStartCapabilityDigestHuman(d startCapabilityDigest) string {
	lines := []string{
		"Flow: " + strings.Join(d.FlowPhases, " -> "),
		"Readiness: MCP=" + d.Readiness.MCP + " | Skills=" + d.Readiness.Skills + " | Hooks=" + d.Readiness.Hooks,
	}
	if d.Remediation != "" {
		lines = append(lines, "Remediation: "+d.Remediation)
	}
	return strings.Join(lines, "\n")
}

func readinessLabel(ok bool) string {
	if ok {
		return "ready"
	}
	return "needs-setup"
}

func isHooksReady(repoRoot string) bool {
	hookStale, _, err := artifactStatus(repoRoot)
	return err == nil && !hookStale
}

func isSkillsReady(repoRoot, adapterID string) bool {
	switch adapterID {
	case "codex":
		return fileExists(filepath.Join(repoRoot, "AGENTS.md"))
	case "claude-code":
		_, claudeStale, err := artifactStatus(repoRoot)
		return err == nil && !claudeStale
	case "gemini":
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		return fileExists(filepath.Join(home, ".gemini", "skills", "docket", "SKILL.md"))
	default:
		return false
	}
}

func isMCPReady(repoRoot string) bool {
	if fileExists(filepath.Join(repoRoot, "doombox.json")) {
		return true
	}
	if fileContains(filepath.Join(repoRoot, ".vscode", "settings.json"), "docket") {
		return true
	}
	if fileContains(filepath.Join(repoRoot, ".cursor", "mcp.json"), "docket") {
		return true
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}
