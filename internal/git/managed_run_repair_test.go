package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepairManagedBranchFromCurrentFastForward(t *testing.T) {
	repo := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "chore: base")

	managedBranch := "docket/TKT-198"
	wtPath := filepath.Join(repo, "wt", "TKT-198")
	if err := CreateWorktree(repo, "TKT-198", managedBranch, wtPath); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	since := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := os.WriteFile(filepath.Join(repo, "ff.txt"), []byte("ff\n"), 0o644); err != nil {
		t.Fatalf("write ff file: %v", err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "feat: link ticket\n\nTicket: TKT-198")

	res, err := RepairManagedBranchFromCurrent(repo, wtPath, managedBranch, "TKT-198", since)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if !res.Repaired || res.Method != "fast-forward" {
		t.Fatalf("expected fast-forward repair, got %+v", res)
	}
	ok, err := HasTicketTrailerSince(repo, managedBranch, "TKT-198", since)
	if err != nil {
		t.Fatalf("validate repaired branch failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected managed branch to include ticket commit after repair")
	}
}

func TestRepairManagedBranchFromCurrentCherryPicksWhenBranchesDiverge(t *testing.T) {
	repo := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "chore: base")

	managedBranch := "docket/TKT-199"
	wtPath := filepath.Join(repo, "wt", "TKT-199")
	if err := CreateWorktree(repo, "TKT-199", managedBranch, wtPath); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	if err := os.WriteFile(filepath.Join(wtPath, "managed.txt"), []byte("managed\n"), 0o644); err != nil {
		t.Fatalf("write managed file: %v", err)
	}
	runGitCmd(t, wtPath, "add", ".")
	runGitCmd(t, wtPath, "commit", "-m", "chore: managed divergence")

	since := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGitCmd(t, repo, "add", ".")
	runGitCmd(t, repo, "commit", "-m", "feat: recover ticket\n\nTicket: TKT-199")

	res, err := RepairManagedBranchFromCurrent(repo, wtPath, managedBranch, "TKT-199", since)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if !res.Repaired || res.Method != "cherry-pick" {
		t.Fatalf("expected cherry-pick repair, got %+v", res)
	}
	ok, err := HasTicketTrailerSince(repo, managedBranch, "TKT-199", since)
	if err != nil {
		t.Fatalf("validate repaired branch failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected managed branch to include ticket commit after cherry-pick")
	}
	log := runGitCmd(t, wtPath, "log", "--format=%B", "-1")
	if !strings.Contains(log, "cherry picked from commit") {
		t.Fatalf("expected cherry-pick annotation in repaired commit, got:\n%s", log)
	}
}
