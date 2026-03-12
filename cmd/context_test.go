package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestContextCmd_HumanJsonAndRange(t *testing.T) {
	repoDir := setupCtxRepo(t)
	repo = repoDir
	contextLines = ""

	s := local.New(repoDir)
	ticket.SaveConfig(repoDir, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Ctx Ticket",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
		Handoff:     "Token validation done.",
		AC:          []ticket.AcceptanceCriterion{{Description: "A", Done: true}, {Description: "B", Done: false}},
	})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	mustWriteCtx(t, filepath.Join(repoDir, "main.go"), "package main\n\nfunc main() {\n\tprintln(\"x\") // [TKT-001] annotation\n}\n")
	runGitCtxCmd(t, repoDir, "add", "main.go", ".docket/tickets/TKT-001.md", ".docket/config.json")
	runGitCtxCmd(t, repoDir, "commit", "-m", "feat: ctx", "-m", "Ticket: TKT-001")

	format = "human"
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"scan"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	buf.Reset()
	rootCmd.SetArgs([]string{"context", "main.go"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("context human failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Context for main.go") || !strings.Contains(out, "Tickets from git history") {
		t.Fatalf("unexpected human output:\n%s", out)
	}
	if !strings.Contains(out, "Inline annotations") {
		t.Fatalf("missing annotations output:\n%s", out)
	}

	buf.Reset()
	format = "context"
	rootCmd.SetArgs([]string{"context", "main.go", "--format", "context", "--lines", "4-4"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("context compact failed: %v", err)
	}
	if !strings.Contains(buf.String(), "FILE: main.go") {
		t.Fatalf("unexpected compact output:\n%s", buf.String())
	}

	buf.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"context", "main.go", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("context json failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	if payload["file"] != "main.go" {
		t.Fatalf("file = %v, want main.go", payload["file"])
	}
}

func TestContextCmd_NoTicketHistory(t *testing.T) {
	repoDir := setupCtxRepo(t)
	repo = repoDir
	format = "human"
	contextLines = ""

	mustWriteCtx(t, filepath.Join(repoDir, "main.go"), "package main\n")
	runGitCtxCmd(t, repoDir, "add", "main.go")
	runGitCtxCmd(t, repoDir, "commit", "-m", "chore: no ticket")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"context", "main.go"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("context failed: %v", err)
	}
	if !strings.Contains(buf.String(), "No tickets linked to this file's history") {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
}

func setupCtxRepo(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	runGitCtxCmd(t, d, "init")
	runGitCtxCmd(t, d, "config", "user.email", "test@example.com")
	runGitCtxCmd(t, d, "config", "user.name", "Test User")
	return d
}

func mustWriteCtx(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func runGitCtxCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
