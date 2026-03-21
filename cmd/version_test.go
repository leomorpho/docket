package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCmdHuman(t *testing.T) {
	prevVersion := Version
	prevCommit := BuildCommit
	prevDate := BuildDate
	prevFormat := format
	t.Cleanup(func() {
		Version = prevVersion
		BuildCommit = prevCommit
		BuildDate = prevDate
		format = prevFormat
	})

	Version = "0.2.0"
	BuildCommit = "abc123"
	BuildDate = "2026-03-21"
	format = "human"

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "docket v0.2.0 (abc123) built 2026-03-21") {
		t.Fatalf("unexpected human output: %s", got)
	}
}

func TestVersionCmdJSON(t *testing.T) {
	prevVersion := Version
	prevCommit := BuildCommit
	prevDate := BuildDate
	prevFormat := format
	t.Cleanup(func() {
		Version = prevVersion
		BuildCommit = prevCommit
		BuildDate = prevDate
		format = prevFormat
	})

	Version = "0.2.0"
	BuildCommit = "abc123"
	BuildDate = "2026-03-21"
	format = "json"

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"version", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json parse failed: %v\n%s", err, out.String())
	}
	if payload["version"] != "v0.2.0" {
		t.Fatalf("expected version v0.2.0, got %#v", payload["version"])
	}
	if payload["commit"] != "abc123" {
		t.Fatalf("expected commit abc123, got %#v", payload["commit"])
	}
}
