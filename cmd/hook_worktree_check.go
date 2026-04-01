package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var hookWorktreeCheckCmd = &cobra.Command{
	Use:    "__hook-worktree-check <TKT-NNN>",
	Short:  "internal commit-time worktree enforcement",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookWorktreeCheck(args[0])
	},
}

func runHookWorktreeCheck(ticketID string) error {
	cwdAbs, err := filepath.Abs(repo)
	if err != nil {
		return fmt.Errorf("resolve current checkout: %w", err)
	}

	gitEntry := filepath.Join(cwdAbs, ".git")
	info, err := os.Stat(gitEntry)
	if err != nil {
		return fmt.Errorf("inspect git checkout: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("ticket %s commit rejected: current checkout is the primary repo; use a dedicated worktree", ticketID)
	}

	s := local.New(repo)
	t, err := s.GetTicket(context.Background(), ticketID)
	if err != nil {
		return fmt.Errorf("load ticket %s: %w", ticketID, err)
	}
	if t == nil {
		return nil
	}

	ns := runstate.New(runtimeNamespaceRoot(repo))
	run, ok, err := ns.GetRunManifest(repo, ticketID)
	if err != nil {
		return fmt.Errorf("read run manifest for %s: %w", ticketID, err)
	}
	if !ok {
		return nil
	}

	expectedAbs, err := filepath.Abs(run.WorktreePath)
	if err != nil {
		return fmt.Errorf("resolve run worktree for %s: %w", ticketID, err)
	}
	if expectedAbs != cwdAbs {
		return fmt.Errorf("ticket %s commit rejected: current worktree %s does not match bound worktree %s", ticketID, cwdAbs, expectedAbs)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(hookWorktreeCheckCmd)
}
