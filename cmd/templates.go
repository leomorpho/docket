package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/leoaudibert/docket/internal/ticket"
	"gopkg.in/yaml.v3"
)

type templateFile struct {
	Items []acDefaultItem `yaml:"ac"`
}

var builtinTemplates = map[string][]ticket.AcceptanceCriterion{
	"api-endpoint": {
		{Description: "Unit tests pass", Run: "go test ./..."},
		{Description: "Integration tests pass"},
		{Description: "OpenAPI docs updated"},
		{Description: "Error handling verified"},
		{Description: "Auth checks verified"},
	},
	"react-component": {
		{Description: "Component unit tests pass", Run: "pnpm test"},
		{Description: "Visual regression checked"},
		{Description: "Accessibility checks pass"},
		{Description: "Storybook story updated"},
	},
	"db-migration": {
		{Description: "Migration up/down succeeds"},
		{Description: "Rollback preserves data"},
		{Description: "Staging migration tested"},
	},
	"cli-command": {
		{Description: "Help text present"},
		{Description: "Happy path test passes"},
		{Description: "Error path test passes"},
		{Description: "CLAUDE.md updated"},
	},
}

func loadUserTemplate(repoRoot, name string) ([]ticket.AcceptanceCriterion, bool) {
	path := filepath.Join(repoRoot, ".docket", "templates", name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var tf templateFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return nil, false
	}
	out := make([]ticket.AcceptanceCriterion, 0, len(tf.Items))
	for _, item := range tf.Items {
		if strings.TrimSpace(item.Desc) == "" {
			continue
		}
		out = append(out, ticket.AcceptanceCriterion{
			Description: strings.TrimSpace(item.Desc),
			Run:         strings.TrimSpace(item.Run),
		})
	}
	return out, true
}

func getTemplate(repoRoot, name string) ([]ticket.AcceptanceCriterion, bool) {
	if u, ok := loadUserTemplate(repoRoot, name); ok {
		return u, true
	}
	b, ok := builtinTemplates[name]
	return b, ok
}

func listTemplates(repoRoot string) []string {
	names := map[string]bool{}
	for k := range builtinTemplates {
		names[k] = true
	}
	userDir := filepath.Join(repoRoot, ".docket", "templates")
	if entries, err := os.ReadDir(userDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			names[strings.TrimSuffix(e.Name(), ".yaml")] = true
		}
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func applyTemplates(repoRoot string, templateArg string) []ticket.AcceptanceCriterion {
	if strings.TrimSpace(templateArg) == "" {
		return nil
	}
	parts := strings.Split(templateArg, ",")
	out := []ticket.AcceptanceCriterion{}
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		items, ok := getTemplate(repoRoot, name)
		if !ok {
			continue
		}
		out = append(out, items...)
	}
	return out
}

func formatTemplate(name string, items []ticket.AcceptanceCriterion) string {
	lines := []string{fmt.Sprintf("Template: %s", name)}
	for i, item := range items {
		line := fmt.Sprintf("%d. %s", i+1, item.Description)
		if item.Run != "" {
			line += fmt.Sprintf(" (run: %s)", item.Run)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
