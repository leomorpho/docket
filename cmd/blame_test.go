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

	"github.com/leoaudibert/docket/internal/store/local"
	"github.com/leoaudibert/docket/internal/ticket"
)

func TestParseFileLineArg(t *testing.T) {
	file, line, err := parseFileLineArg("main.go:42")
	if err != nil {
		t.Fatalf("parseFileLineArg failed: %v", err)
	}
	if file != "main.go" || line != 42 {
		t.Fatalf("unexpected parse result: %q, %d", file, line)
	}

	if _, _, err := parseFileLineArg("main.go"); err == nil {
		t.Fatal("expected format error")
	}
	if _, _, err := parseFileLineArg("main.go:x"); err == nil {
		t.Fatal("expected line parse error")
	}
}

func TestBlameCmd_TicketFoundAndJSON(t *testing.T) {
	repoDir := setupBlameRepo(t)
	repo = repoDir

	s := local.New(repoDir)
	ticket.SaveConfig(repoDir, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		Title:       "Blame linked ticket",
		State:       ticket.State("in-progress"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "human:test",
		Description: "desc",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	file := filepath.Join(repoDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main(){\n\tprintln(\"x\")\n}\n"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	runGitForBlame(t, repoDir, "add", "main.go", ".docket/tickets/TKT-001.md", ".docket/config.json")
	runGitForBlame(t, repoDir, "commit", "-m", "feat: add source", "-m", "Ticket: TKT-001")

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	format = "human"
	rootCmd.SetArgs([]string{"blame", "main.go:4"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("blame cmd failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "TKT-001 · in-progress") {
		t.Fatalf("expected ticket header, got:\n%s", out)
	}
	if !strings.Contains(out, "This line was last modified in commit") {
		t.Fatalf("expected commit context line, got:\n%s", out)
	}

	b.Reset()
	format = "json"
	rootCmd.SetArgs([]string{"blame", "main.go:4", "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("blame json failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &payload); err != nil {
		t.Fatalf("json parse failed: %v", err)
	}
	if payload["ticket_id"] != "TKT-001" {
		t.Fatalf("ticket_id = %v, want TKT-001", payload["ticket_id"])
	}
	if payload["ticket"] == nil {
		t.Fatal("expected ticket object in json response")
	}
}

func TestBlameCmd_NoTicketAndErrors(t *testing.T) {
	repoDir := setupBlameRepo(t)
	repo = repoDir
	format = "human"

	file := filepath.Join(repoDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main(){\n\tprintln(\"x\")\n}\n"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	runGitForBlame(t, repoDir, "add", "main.go")
	runGitForBlame(t, repoDir, "commit", "-m", "refactor: cleanup")

	b := new(bytes.Buffer)
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"blame", "main.go:4"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("blame no-ticket should succeed: %v", err)
	}
	if !strings.Contains(b.String(), "No ticket linked to commit") {
		t.Fatalf("expected no-ticket output, got:\n%s", b.String())
	}

	rootCmd.SetArgs([]string{"blame", "nonexistent.go:1"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for missing file")
	}

	rootCmd.SetArgs([]string{"blame", "main.go:99999"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for out of range line")
	}
}

func setupBlameRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitForBlame(t, dir, "init")
	runGitForBlame(t, dir, "config", "user.email", "test@example.com")
	runGitForBlame(t, dir, "config", "user.name", "Test User")
	return dir
}

func runGitForBlame(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
