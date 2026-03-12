package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestEnsureLocksGitignored(t *testing.T) {
	tmp := t.TempDir()
	if err := ensureLocksGitignored(tmp); err != nil {
		t.Fatalf("ensureLocksGitignored failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore failed: %v", err)
	}
	if !strings.Contains(string(data), ".docket/locks.json") {
		t.Fatalf("expected locks gitignore entry, got: %s", string(data))
	}
}

func TestUpdateAutoReleasesLockOnDoneState(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	s := local.New(tmp)
	if err := ticket.SaveConfig(tmp, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-010", Seq: 10, Title: "Lock release", State: ticket.State("todo"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := upsertLock(tmp, fileLock{TicketID: "TKT-010", WorktreePath: tmp, Files: []string{"x.go"}, UpdatedAt: now.Format(time.RFC3339)}); err != nil {
		t.Fatalf("upsert lock: %v", err)
	}

	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-010", "--state", "in-progress"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update in-progress failed: %v", err)
	}
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"update", "TKT-010", "--state", "in-review"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("update in-review failed: %v", err)
	}

	st, err := loadLocks(tmp)
	if err != nil {
		t.Fatalf("load locks failed: %v", err)
	}
	if len(st.Locks) != 0 {
		t.Fatalf("expected lock auto-release, got %+v", st.Locks)
	}
}

func TestHookLockCheckWarnsOnOverlap(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	_ = os.MkdirAll(filepath.Join(tmp, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, ".git", "COMMIT_EDITMSG"), []byte("feat: x\n\nTicket: TKT-999\n"), 0o644)

	// Initialize git repo for staged-files detection.
	run := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmp}, args...)...)
		_ = c.Run()
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	_ = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644)
	run("add", "a.txt")
	run("commit", "-m", "init")
	_ = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("b"), 0o644)
	run("add", "a.txt")

	s := local.New(tmp)
	_ = ticket.SaveConfig(tmp, ticket.DefaultConfig())
	now := time.Now().UTC().Truncate(time.Second)
	_ = s.CreateTicket(context.Background(), &ticket.Ticket{
		ID: "TKT-777", Seq: 777, Title: "Other lock", State: ticket.State("in-progress"), Priority: 1,
		CreatedAt: now, UpdatedAt: now, CreatedBy: "me", Description: "desc", AC: []ticket.AcceptanceCriterion{{Description: "A"}},
	})
	_ = upsertLock(tmp, fileLock{TicketID: "TKT-777", WorktreePath: tmp, Files: []string{"a.txt"}, UpdatedAt: now.Format(time.RFC3339)})

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"__hook-lock-check"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("hook lock check failed: %v", err)
	}
	if !strings.Contains(errBuf.String(), "overlap") {
		t.Fatalf("expected overlap warning, got: %s", errBuf.String())
	}
}

func TestWorktreeStartAndLockStatus(t *testing.T) {
	tmp := t.TempDir()
	repo = tmp
	format = "human"
	run := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", tmp}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, strings.TrimSpace(string(out)))
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	_ = os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644)
	run("add", "a.txt")
	run("commit", "-m", "init")

	worktreePath := filepath.Join(tmp, "..", "wt-lock-test")
	_ = os.RemoveAll(worktreePath)
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"worktree", "start", "TKT-888", worktreePath})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("worktree start failed: %v", err)
	}
	st, err := loadLocks(tmp)
	if err != nil {
		t.Fatalf("load locks failed: %v", err)
	}
	if len(st.Locks) != 1 || st.Locks[0].TicketID != "TKT-888" {
		t.Fatalf("expected lock registration, got %+v", st.Locks)
	}
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"lock", "status"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("lock status failed: %v", err)
	}
	if !strings.Contains(out.String(), "TKT-888") {
		t.Fatalf("expected lock status to include TKT-888, got: %s", out.String())
	}

	_ = os.RemoveAll(worktreePath)
}
