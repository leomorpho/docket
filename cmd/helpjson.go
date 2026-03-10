package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var Version = "0.1.0"

var helpJSONCmd = &cobra.Command{
	Use:   "help-json",
	Short: "Print machine-readable CLI manifest",
	RunE: func(cmd *cobra.Command, args []string) error {
		manifest := map[string]interface{}{
			"binary":      "docket",
			"version":     Version,
			"description": rootCmd.Short,
			"global_flags": map[string]interface{}{
				"--format": map[string]interface{}{"type": "string", "values": []string{"human", "json", "context", "md"}, "default": "human"},
				"--repo":   map[string]interface{}{"type": "string", "default": "current working directory"},
			},
			"commands":    buildCommandManifest(rootCmd),
			"environment": map[string]string{"DOCKET_ACTOR": "Set actor identity, e.g. 'agent:claude-sonnet-4-6'. Falls back to git config user.name."},
			"conventions": map[string]string{
				"ticket_id_format":  "TKT-NNN (e.g. TKT-001, TKT-042)",
				"commit_trailer":    "Add 'Ticket: TKT-NNN' to commit messages to link work",
				"inline_annotation": "Add '// [TKT-NNN] reason' in source code for explicit markers",
				"actor_format":      "'human:name' or 'agent:model-id'",
			},
		}
		printJSON(cmd, manifest)
		return nil
	},
}

func buildCommandManifest(root *cobra.Command) []map[string]interface{} {
	entries := []map[string]interface{}{}

	var walk func(parentPath string, c *cobra.Command)
	walk = func(parentPath string, c *cobra.Command) {
		if c == root {
			for _, child := range c.Commands() {
				walk("", child)
			}
			return
		}
		if c.Hidden {
			return
		}

		full := strings.TrimSpace(strings.Join([]string{parentPath, c.Name()}, " "))
		entry := map[string]interface{}{
			"name":        full,
			"synopsis":    fmt.Sprintf("docket %s", c.Use),
			"description": c.Short,
			"flags":       commandFlags(c),
			"examples":    commandExamples(full),
			"output":      commandOutputShape(full),
		}
		entries = append(entries, entry)

		for _, child := range c.Commands() {
			walk(full, child)
		}
	}

	walk("", root)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i]["name"].(string) < entries[j]["name"].(string)
	})
	return entries
}

func commandFlags(c *cobra.Command) map[string]interface{} {
	flags := map[string]interface{}{}
	c.Flags().VisitAll(func(f *pflag.Flag) {
		flags["--"+f.Name] = map[string]interface{}{
			"type":    f.Value.Type(),
			"default": f.DefValue,
		}
	})
	return flags
}

func commandExamples(name string) []string {
	examples := map[string][]string{
		"create":           {"docket create --title 'Add auth middleware' --priority 1 --labels feature", "echo 'Long description' | docket create --title 'Fix bug' --desc -"},
		"list":             {"docket list --state open", "docket list --format json"},
		"show":             {"docket show TKT-001", "docket show TKT-001 --format context"},
		"update":           {"docket update TKT-001 --state in-progress", "docket update TKT-001 --priority 1"},
		"comment":          {"docket comment TKT-001 --body 'Decision details'"},
		"blame":            {"docket blame main.go:42"},
		"scan":             {"docket scan", "docket scan --path internal"},
		"refs":             {"docket refs TKT-001"},
		"context":          {"docket context internal/auth/middleware.go", "docket context main.go --lines 1-40 --format context"},
		"session attach":   {"docket session attach TKT-001 --file /tmp/log.jsonl"},
		"session list":     {"docket session list TKT-001"},
		"session compress": {"docket session compress TKT-001", "docket session compress TKT-001 --summary-file /tmp/summary.md"},
		"ac add":           {"docket ac add TKT-001 --desc 'Tests pass'"},
		"ac complete":      {"docket ac complete TKT-001 --step 1 --evidence 'go test passed'"},
		"ac check":         {"docket ac check TKT-001", "docket ac check TKT-001 --format json"},
		"ac list":          {"docket ac list TKT-001"},
		"check":            {"docket check", "docket check TKT-001 --fix"},
		"help-json":        {"docket help-json | jq .commands[].name"},
	}
	if v, ok := examples[name]; ok {
		return v
	}
	return []string{fmt.Sprintf("docket %s", name)}
}

func commandOutputShape(name string) map[string]interface{} {
	shapes := map[string]map[string]interface{}{
		"create":    {"human": "Created TKT-001: <title>", "json": map[string]string{"id": "string", "seq": "int", "title": "string", "state": "string"}},
		"list":      {"human": "table/lines", "json": "array of tickets"},
		"show":      {"human": "expanded ticket", "json": "ticket object", "context": "compact ticket context"},
		"check":     {"human": "findings summary", "json": map[string]string{"checked": "int", "findings": "array", "summary": "object"}},
		"help-json": {"json": "manifest object"},
	}
	if v, ok := shapes[name]; ok {
		return v
	}
	return map[string]interface{}{"human": "command output", "json": "when --format json is supported"}
}

func init() {
	rootCmd.AddCommand(helpJSONCmd)
}
