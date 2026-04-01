package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestHookWorktreeCheckRejectsPrimaryCheckoutForTicketCommit(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-910", 910, ticket.State("running"), updateRunnableAC())

	err := runHookWorktreeCheck("TKT-910")
	if err == nil || !strings.Contains(err.Error(), "primary repo") {
		t.Fatalf("expected primary checkout rejection, got: %v", err)
	}
}

func TestHookWorktreeCheckAcceptsBoundDedicatedWorktree(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-911", 911, ticket.State("running"), updateRunnableAC())

	wtPath := filepath.Join(t.TempDir(), "wt-TKT-911")
	runGitSession(t, h.repo, "worktree", "add", "-b", "docket/TKT-911-test", wtPath)

	ns := runstate.New(defaultRuntimeNamespaceRoot(h.repo))
	if err := ns.RecordRunStart(h.repo, "TKT-911", "human:test", wtPath, "docket/TKT-911-test", ""); err != nil {
		t.Fatalf("record run manifest failed: %v", err)
	}

	oldRepo := repo
	repo = wtPath
	t.Cleanup(func() { repo = oldRepo })

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(wtPath); err != nil {
		t.Fatalf("chdir worktree failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldDir)
	})

	if err := runHookWorktreeCheck("TKT-911"); err != nil {
		t.Fatalf("expected dedicated worktree check to pass, got: %v", err)
	}
}
