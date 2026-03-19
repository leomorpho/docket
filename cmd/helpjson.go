package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var helpJSONCmd = &cobra.Command{
	Use:   "help-json",
	Short: "Print machine-readable CLI manifest",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			cfg = ticket.DefaultConfig()
		}
		manifest := map[string]any{
			"binary":      "docket",
			"version":     Version,
			"description": rootCmd.Short,
			"agent_instructions": map[string]any{
				"file_access": "Prefer docket CLI commands for ticket reads because they expose computed fields (AC status, linked files, state history) that raw files do not include. Direct edits to .docket/tickets/*.md are allowed when needed, but you must run `docket validate TKT-NNN` or `docket validate` afterward before committing.",
				"ticket_quality": map[string]any{
					"size":        "Keep tickets atomic — one deliverable completable in a single focused session. If a task touches more than 3 files or has multiple logical phases, split it into child tickets under a parent epic using --parent TKT-NNN.",
					"description": "The description must explain context, constraints, and the 'why' — not just restate the title. A cheap LLM should be able to pick up the ticket and execute it without asking clarifying questions.",
					"ac":          "Every ticket needs at least 2 acceptance criteria that are specific and testable. Add them immediately after creation: docket ac add TKT-NNN --desc 'specific observable outcome'",
					"comments":    "Add a comment for every significant decision made during work: docket comment TKT-NNN --body 'Chose X over Y because...'",
				},
				"workflow": map[string]any{
					"start":                 "docket list --state open --format context",
					"pick_up":               "docket show TKT-NNN --format context",
					"search":                "docket search \"query\"",
					"work":                  fmt.Sprintf("docket update TKT-NNN --state %s", activeWorkflowState(cfg)),
					"finish":                fmt.Sprintf("docket update TKT-NNN --state %s && docket session compress TKT-NNN", reviewWorkflowState(cfg)),
					"quick_path_preference": "Prefer transactional authoring via scaffold/apply commands over multi-step manual edits.",
					"ticket_apply":          "docket ticket scaffold > ticket-spec.json && docket --automation ticket apply --spec ticket-spec.json",
					"backlog_apply":         "docket backlog scaffold > backlog-spec.json && docket --automation backlog apply --spec backlog-spec.json",
					"proof_attach":          "docket proof add TKT-NNN --file artifacts/screenshot.png --proof-title \"Before fix\" --note \"What this screenshot proves\" --captured-at 2026-03-16T18:40:00Z --format json",
					"proof_verify":          "docket proof list TKT-NNN --format json && docket show TKT-NNN --format json",
				},
			},
			"global_flags": map[string]any{
				"--format": map[string]any{"type": "string", "values": []string{"human", "json", "context", "md"}, "default": "human"},
				"--repo":   map[string]any{"type": "string", "default": "current working directory"},
			},
			"commands": nil,
			"environment": map[string]string{
				"DOCKET_ACTOR":      "Set actor identity, e.g. 'agent:claude-sonnet-4-6'. Falls back to git config user.name.",
				"DOCKET_AUTOMATION": "Set to 1 to force non-interactive deterministic automation behavior.",
			},
			"conventions": map[string]string{
				"ticket_id_format":  "TKT-NNN (e.g. TKT-001, TKT-042)",
				"commit_trailer":    "Add 'Ticket: TKT-NNN' to commit messages to link work",
				"inline_annotation": "Add '// [TKT-NNN] reason' in source code for explicit markers",
				"actor_format":      "'human:name' or 'agent:model-id'",
			},
		}
		manifest["commands"] = buildCommandManifest(rootCmd, cfg)
		printJSON(cmd, manifest)
		return nil
	},
}

func buildCommandManifest(root *cobra.Command, cfg *ticket.Config) []map[string]any {
	entries := []map[string]any{}

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
		entry := map[string]any{
			"name":        full,
			"synopsis":    fmt.Sprintf("docket %s", c.Use),
			"description": c.Short,
			"flags":       commandFlags(c),
			"examples":    commandExamples(full, cfg),
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

func commandFlags(c *cobra.Command) map[string]any {
	flags := map[string]any{}
	c.Flags().VisitAll(func(f *pflag.Flag) {
		flags["--"+f.Name] = map[string]any{
			"type":    f.Value.Type(),
			"default": f.DefValue,
		}
	})
	return flags
}

func commandExamples(name string, cfg *ticket.Config) []string {
	examples := map[string][]string{
		"create":           {"docket create --title 'Add auth middleware' --priority 1 --labels feature", "echo 'Long description' | docket create --title 'Fix bug' --desc -"},
		"list":             {"docket list --state open", "docket list --format json"},
		"show":             {"docket show TKT-001", "docket show TKT-001 --format context"},
		"search":           {"docket search \"auth middleware\"", "docket search \"token validation\" --semantic auto"},
		"update":           {fmt.Sprintf("docket update TKT-001 --state %s", activeWorkflowState(cfg)), "docket update TKT-001 --priority 1"},
		"comment":          {"docket comment TKT-001 --body 'Decision details'"},
		"blame":            {"docket blame main.go:42"},
		"scan":             {"docket scan", "docket scan --path internal"},
		"refs":             {"docket refs TKT-001"},
		"context":          {"docket context internal/auth/middleware.go", "docket context main.go --lines 1-40 --format context"},
		"session attach":   {"docket session attach TKT-001 --file /tmp/log.jsonl"},
		"session list":     {"docket session list TKT-001"},
		"session compress": {"docket session compress TKT-001", "docket session compress TKT-001 --summary-file /tmp/summary.md"},
		"ac add":           {"docket ac add TKT-001 --desc 'Tests pass'", "docket ac add TKT-001 --desc 'Unit tests pass' --run 'go test ./...'"},
		"ac complete":      {"docket ac complete TKT-001 --step 1 --evidence 'go test passed'"},
		"ac check":         {"docket ac check TKT-001", "docket ac check TKT-001 --dry-run"},
		"ac list":          {"docket ac list TKT-001"},
		"skill list":       {"docket skill list", "docket skill list --format json"},
		"skill show":       {"docket skill show ticket-discovery"},
		"skill invoke":     {"docket skill invoke ticket-discovery", "docket skill invoke learning-replay --ticket TKT-001"},
		"skill audit":      {"docket skill audit", "docket skill audit learning-replay --bucket day --format json"},
		"hook list":        {"docket hook list", "docket hook list --format json"},
		"hook show":        {"docket hook show ticket.review"},
		"hook status":      {"docket hook status", "docket hook status --format json"},
		"smart-commit":     {"docket smart-commit TKT-001", "docket smart-commit TKT-001 --validate \"feat: x\\n\\nTicket: TKT-001\""},
		"context-optimize": {"docket context-optimize TKT-001", "docket context-optimize TKT-001 --format json"},
		"check":            {"docket check", "docket check TKT-001 --fix"},
		"install":          {"docket install"},
		"upgrade":          {"docket upgrade", "docket upgrade --check"},
		"workflow pack":    {"docket workflow pack", "docket workflow pack --output .docket/instruction-pack.json"},
		"wrap-up":          {"docket wrap-up TKT-001", "docket wrap-up TKT-001 --format json"},
		"help-json":        {"docket help-json | jq .commands[].name"},
	}
	if v, ok := examples[name]; ok {
		return v
	}
	return []string{fmt.Sprintf("docket %s", name)}
}

func commandOutputShape(name string) map[string]any {
	shapes := map[string]map[string]any{
		"create":           {"human": "Created TKT-001: <title>", "json": map[string]string{"id": "string", "seq": "int", "title": "string", "state": "string"}},
		"list":             {"human": "table/lines", "json": "array of tickets"},
		"show":             {"human": "expanded ticket", "json": "ticket object", "context": "compact ticket context"},
		"skill list":       {"human": "skill inventory with metadata", "json": map[string]string{"total": "int", "metadata_checksum": "string", "skills": "array"}},
		"skill show":       {"human": "single skill metadata", "json": map[string]string{"skill": "object", "metadata_checksum": "string"}},
		"skill invoke":     {"human": "resolved invocation command", "json": map[string]string{"skill_id": "string", "ticket_id": "string", "command": "string", "intent": "string"}},
		"skill audit":      {"human": "skill usage totals and timeline", "json": map[string]string{"total_invocations": "int", "bucket_size": "string", "skills": "array", "timeline": "array"}},
		"hook list":        {"human": "hook events and modes", "json": map[string]string{"total": "int", "events": "array"}},
		"hook show":        {"human": "single hook event detail", "json": map[string]string{"event": "object", "namespace": "string", "invocation": "string", "execution": "string"}},
		"hook status":      {"human": "hook readiness and metadata", "json": map[string]string{"ready": "bool", "readiness": "string", "events": "array"}},
		"context-optimize": {"human": "compact ticket brief from related work, learnings, and recent activity", "json": map[string]string{"ticket_id": "string", "brief": "string", "related_work": "array", "learning_rules": "array", "recent_activity": "object", "next_steps": "array"}},
		"smart-commit":     {"human": "review-ready commit guidance with ticket trailer", "json": map[string]string{"ticket_id": "string", "ready": "bool", "commit_message": "string", "git_command": "string"}},
		"check":            {"human": "findings summary", "json": map[string]string{"checked": "int", "findings": "array", "summary": "object"}},
		"wrap-up":          {"human": "ticket readiness summary with next steps", "json": map[string]string{"ticket_id": "string", "ready": "bool", "checks": "array", "next_steps": "array"}},
		"help-json":        {"json": "manifest object"},
	}
	if v, ok := shapes[name]; ok {
		return v
	}
	return map[string]any{"human": "command output", "json": "when --format json is supported"}
}

func init() {
	rootCmd.AddCommand(helpJSONCmd)
}
