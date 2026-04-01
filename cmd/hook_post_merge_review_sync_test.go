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

func TestHookPostMergeReviewSync_DoesNotMutateMergedTicketState(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepoForHookSync(t, repoRoot)
	writeAndCommitFile(t, repoRoot, "seed.txt", "seed\n", "chore: seed")

	wtPath := filepath.Join(t.TempDir(), "wt-401")
	runGitHookSync(t, repoRoot, "worktree", "add", "-b", "docket/TKT-401", wtPath)
	writeAndCommitFile(t, wtPath, "feature.txt", "feature\n", "feat: implement\n\nTicket: TKT-401")
	runGitHookSync(t, repoRoot, "merge", "--no-ff", "-m", "Merge ticket branch docket/TKT-401", "docket/TKT-401")

	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s := local.New(repoRoot)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-401",
		Seq:         401,
		Title:       "Merged leaf",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "test",
		Description: updateRunnableDescription(),
		AC:          updateCompletedAC(),
		Handoff:     updateStructuredHandoff("running", "none"),
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	var stderr bytes.Buffer
	if err := runHookPostMergeReviewSync(context.Background(), repoRoot, &stderr); err != nil {
		t.Fatalf("hook sync failed: %v", err)
	}
	got, err := s.GetTicket(context.Background(), "TKT-401")
	if err != nil {
		t.Fatalf("reload ticket failed: %v", err)
	}
	if got == nil || got.State != ticket.State("running") {
		t.Fatalf("expected TKT-401 unchanged after merge sync, got %#v", got)
	}
	if !strings.Contains(stderr.String(), "post-merge review sync is disabled for TKT-401") {
		t.Fatalf("expected legacy no-op log, got: %s", stderr.String())
	}
}

func TestHookPostMergeReviewSync_NoOpWhenHeadIsNotMergeCommit(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepoForHookSync(t, repoRoot)
	writeAndCommitFile(t, repoRoot, "seed.txt", "seed\n", "chore: seed")

	if err := ticket.SaveConfig(repoRoot, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s := local.New(repoRoot)
	if err := s.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-402",
		Seq:         402,
		Title:       "No merge",
		State:       ticket.State("running"),
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "test",
		Description: updateRunnableDescription(),
		AC:          updateRunnableAC(),
	}); err != nil {
		t.Fatalf("create ticket failed: %v", err)
	}

	var stderr bytes.Buffer
	if err := runHookPostMergeReviewSync(context.Background(), repoRoot, &stderr); err != nil {
		t.Fatalf("hook sync failed: %v", err)
	}
	got, err := s.GetTicket(context.Background(), "TKT-402")
	if err != nil {
		t.Fatalf("reload ticket failed: %v", err)
	}
	if got == nil || got.State != ticket.State("running") {
		t.Fatalf("expected TKT-402 unchanged, got %#v", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no hook output for non-merge head, got: %s", stderr.String())
	}
}

func initGitRepoForHookSync(t *testing.T, repoRoot string) {
	t.Helper()
	runGitHookSync(t, repoRoot, "init")
	runGitHookSync(t, repoRoot, "config", "user.email", "test@example.com")
	runGitHookSync(t, repoRoot, "config", "user.name", "Test User")
}

func writeAndCommitFile(t *testing.T, repoRoot, relPath, content, message string) {
	t.Helper()
	path := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	runGitHookSync(t, repoRoot, "add", relPath)
	runGitHookSync(t, repoRoot, "commit", "-m", message)
}

func runGitHookSync(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
	}
}
