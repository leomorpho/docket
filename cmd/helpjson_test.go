package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestHelpJSONCommand(t *testing.T) {
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"help-json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("help-json failed: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(b.Bytes(), &manifest); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if manifest["binary"] != "docket" {
		t.Fatalf("binary = %v, want docket", manifest["binary"])
	}
	if _, ok := manifest["agent_instructions"].(map[string]any); !ok {
		t.Fatalf("missing agent_instructions in manifest")
	}
	ai := manifest["agent_instructions"].(map[string]any)
	quality, ok := ai["ticket_quality"].(map[string]any)
	if !ok {
		t.Fatalf("ticket_quality guidance missing in agent_instructions")
	}
	for _, key := range []string{"size", "description", "ac", "comments"} {
		if quality[key] == nil {
			t.Fatalf("ticket_quality.%s missing", key)
		}
	}
	workflow, ok := ai["workflow"].(map[string]any)
	if !ok {
		t.Fatalf("workflow guidance missing in agent_instructions")
	}
	for _, key := range []string{"quick_path_preference", "ticket_apply", "backlog_apply", "proof_attach", "proof_verify"} {
		if workflow[key] == nil {
			t.Fatalf("workflow.%s missing", key)
		}
	}
	if !strings.Contains(workflow["ticket_apply"].(string), "--automation") {
		t.Fatalf("ticket_apply guidance must include automation mode hint: %v", workflow["ticket_apply"])
	}
	if !strings.Contains(workflow["proof_attach"].(string), "proof add") || !strings.Contains(workflow["proof_attach"].(string), "--proof-title") || !strings.Contains(workflow["proof_attach"].(string), "--note") {
		t.Fatalf("proof_attach guidance must include canonical proof add command: %v", workflow["proof_attach"])
	}
	if !strings.Contains(workflow["proof_verify"].(string), "proof list") || !strings.Contains(workflow["proof_verify"].(string), "show") {
		t.Fatalf("proof_verify guidance must include list + show validation path: %v", workflow["proof_verify"])
	}

	commands, ok := manifest["commands"].([]any)
	if !ok || len(commands) == 0 {
		t.Fatalf("commands missing or empty")
	}

	names := map[string]bool{}
	for _, c := range commands {
		m := c.(map[string]any)
		names[m["name"].(string)] = true
	}

	required := []string{"create", "list", "show", "update", "comment", "board", "blame", "scan", "refs", "context", "session", "ac", "skill", "hook", "smart-commit", "check", "help-json", "install", "upgrade"}
	for _, r := range required {
		if !names[r] {
			t.Fatalf("missing command in manifest: %s", r)
		}
	}
	if !names["context-optimize"] {
		t.Fatalf("missing command in manifest: context-optimize")
	}
	for name := range names {
		if strings.HasPrefix(name, "__hook-") {
			t.Fatalf("internal hook command leaked into public manifest: %s", name)
		}
	}

	var contextOptimize map[string]any
	for _, c := range commands {
		m := c.(map[string]any)
		if m["name"] == "context-optimize" {
			contextOptimize = m
			break
		}
	}
	if contextOptimize == nil {
		t.Fatal("expected manifest entry for context-optimize")
	}
	examples, ok := contextOptimize["examples"].([]any)
	if !ok || len(examples) == 0 {
		t.Fatalf("expected examples for context-optimize, got %#v", contextOptimize["examples"])
	}
	if got := examples[0].(string); !strings.Contains(got, "docket context-optimize TKT-001") {
		t.Fatalf("unexpected context-optimize example: %s", got)
	}
	output, ok := contextOptimize["output"].(map[string]any)
	if !ok {
		t.Fatalf("expected output shape for context-optimize, got %#v", contextOptimize["output"])
	}
	if output["json"] == nil {
		t.Fatalf("expected json output shape for context-optimize, got %#v", output)
	}

	env, ok := manifest["environment"].(map[string]any)
	if !ok || env["DOCKET_ACTOR"] == nil || env["DOCKET_AUTOMATION"] == nil {
		t.Fatalf("environment guidance missing DOCKET_ACTOR or DOCKET_AUTOMATION")
	}

	conv, ok := manifest["conventions"].(map[string]any)
	if !ok || conv["commit_trailer"] == nil || conv["inline_annotation"] == nil {
		t.Fatalf("conventions section missing required keys")
	}
}

func TestHelpJSONWorkflowUsesConfiguredRoleStates(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "json"

	cfg := ticket.DefaultConfig()
	cfg.States = map[string]ticket.StateConfig{
		"queued":   {Label: "Queued", Open: true, Column: 0, Next: []string{"building"}, Roles: []string{"intake"}, Startable: true, BlocksDependents: true},
		"building": {Label: "Building", Open: true, Column: 1, Next: []string{"qa", "queued"}, Roles: []string{"active"}, BlocksDependents: true},
		"qa":       {Label: "QA", Open: true, Column: 2, Next: []string{"shipped", "building"}, Roles: []string{"review"}, Reviewable: true, BlocksDependents: true},
		"shipped":  {Label: "Shipped", Open: false, Column: 3, Next: []string{}, Roles: []string{"completed"}, Terminal: true},
	}
	cfg.DefaultState = "queued"
	if err := ticket.SaveConfig(tmp, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"help-json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("help-json failed: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(b.Bytes(), &manifest); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	workflow := manifest["agent_instructions"].(map[string]any)["workflow"].(map[string]any)
	if got := workflow["work"].(string); !strings.Contains(got, "--state building") {
		t.Fatalf("expected configured active state in workflow guidance, got %q", got)
	}
	if got := workflow["finish"].(string); !strings.Contains(got, "--state shipped") {
		t.Fatalf("expected configured completed state in workflow guidance, got %q", got)
	}
}

func TestHelpJSONStatusManifestOmitsParallelFlag(t *testing.T) {
	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"help-json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("help-json failed: %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(b.Bytes(), &manifest); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	commands, ok := manifest["commands"].([]any)
	if !ok {
		t.Fatalf("commands missing from manifest")
	}
	for _, c := range commands {
		entry := c.(map[string]any)
		if entry["name"] != "status" {
			continue
		}
		flags, ok := entry["flags"].(map[string]any)
		if !ok {
			t.Fatalf("status flags missing from manifest entry")
		}
		if _, exists := flags["--parallel"]; exists {
			t.Fatalf("status manifest should not expose retired --parallel flag")
		}
		if strings.Contains(strings.ToLower(entry["description"].(string)), "parallel") {
			t.Fatalf("status description should not advertise parallel safety, got %q", entry["description"])
		}
		return
	}
	t.Fatal("status command missing from manifest")
}
