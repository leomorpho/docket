package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
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

	commands, ok := manifest["commands"].([]any)
	if !ok || len(commands) == 0 {
		t.Fatalf("commands missing or empty")
	}

	names := map[string]bool{}
	for _, c := range commands {
		m := c.(map[string]any)
		names[m["name"].(string)] = true
	}

	required := []string{"create", "list", "show", "update", "comment", "board", "blame", "scan", "refs", "context", "session", "ac", "check", "help-json"}
	for _, r := range required {
		if !names[r] {
			t.Fatalf("missing command in manifest: %s", r)
		}
	}

	env, ok := manifest["environment"].(map[string]any)
	if !ok || env["DOCKET_ACTOR"] == nil {
		t.Fatalf("environment DOCKET_ACTOR missing")
	}

	conv, ok := manifest["conventions"].(map[string]any)
	if !ok || conv["commit_trailer"] == nil || conv["inline_annotation"] == nil {
		t.Fatalf("conventions section missing required keys")
	}
}
