package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/leoaudibert/docket/internal/ticket"
	"gopkg.in/yaml.v3"
)

type acDefaultItem struct {
	Desc string `yaml:"desc"`
	Run  string `yaml:"run"`
}

type acDefaultsConfig struct {
	ACDefaults map[string][]acDefaultItem `yaml:"ac_defaults"`
}

func detectStack(repoRoot string) string {
	if exists(filepath.Join(repoRoot, "package.json")) {
		if data, err := os.ReadFile(filepath.Join(repoRoot, "package.json")); err == nil {
			raw := strings.ToLower(string(data))
			if strings.Contains(raw, "\"typescript\"") || strings.Contains(raw, "\"ts-node\"") {
				return "typescript"
			}
		}
		return "javascript"
	}
	if exists(filepath.Join(repoRoot, "pyproject.toml")) || exists(filepath.Join(repoRoot, "requirements.txt")) {
		return "python"
	}
	if exists(filepath.Join(repoRoot, "go.mod")) {
		return "go"
	}
	if exists(filepath.Join(repoRoot, "Cargo.toml")) {
		return "rust"
	}
	return ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func builtinACDefaults(stack string) []ticket.AcceptanceCriterion {
	switch stack {
	case "typescript":
		return []ticket.AcceptanceCriterion{
			{Description: "TypeScript compiles without errors", Run: "pnpm tsc --noEmit"},
			{Description: "Tests pass", Run: "pnpm test"},
			{Description: "Coverage check passes", Run: "pnpm test --coverage"},
		}
	case "javascript":
		return []ticket.AcceptanceCriterion{
			{Description: "Tests pass", Run: "pnpm test"},
			{Description: "Coverage check passes", Run: "pnpm test --coverage"},
		}
	case "python":
		return []ticket.AcceptanceCriterion{
			{Description: "Python tests pass", Run: "pytest"},
			{Description: "Coverage meets threshold", Run: "pytest --cov --cov-fail-under=80"},
		}
	case "go":
		return []ticket.AcceptanceCriterion{
			{Description: "Go tests pass", Run: "go test ./..."},
		}
	case "rust":
		return []ticket.AcceptanceCriterion{
			{Description: "Rust tests pass", Run: "cargo test"},
		}
	default:
		return nil
	}
}

func configACDefaults(repoRoot, stack string) ([]ticket.AcceptanceCriterion, bool) {
	configPath := filepath.Join(repoRoot, ".docket", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false
	}
	var cfg acDefaultsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}
	items, ok := cfg.ACDefaults[stack]
	if !ok {
		return nil, false
	}
	out := make([]ticket.AcceptanceCriterion, 0, len(items))
	for _, item := range items {
		desc := strings.TrimSpace(item.Desc)
		if desc == "" {
			continue
		}
		out = append(out, ticket.AcceptanceCriterion{
			Description: desc,
			Run:         strings.TrimSpace(item.Run),
		})
	}
	return out, true
}

func autoACDefaults(repoRoot string) []ticket.AcceptanceCriterion {
	stack := detectStack(repoRoot)
	if stack == "" {
		return nil
	}
	if configured, ok := configACDefaults(repoRoot, stack); ok {
		return configured
	}
	return builtinACDefaults(stack)
}
