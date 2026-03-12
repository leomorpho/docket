package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage ticket worktrees and scope locks",
}
var worktreeForce bool

var worktreeStartCmd = &cobra.Command{
	Use:   "start <TKT-NNN> <path>",
	Short: "Create a git worktree and register ticket lock claims",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ticketID := args[0]
		path := args[1]

		relations, _ := loadRelations(repo)
		for _, rel := range relations.Relations {
			if rel.Relation == "blocks" && rel.To == ticketID && activeInProgress(repo, rel.From) && !worktreeForce {
				return fmt.Errorf("%s is blocked by in-progress ticket %s; rerun with --force to continue", ticketID, rel.From)
			}
		}

		branch := fmt.Sprintf("docket/%s-%d", strings.ToLower(ticketID), time.Now().Unix())
		c := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path)
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add failed: %s", strings.TrimSpace(string(out)))
		}

		lock := fileLock{
			TicketID:     ticketID,
			WorktreePath: path,
			Files:        claimFilesForWorktree(path),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		}
		if err := upsertLock(repo, lock); err != nil {
			return err
		}
		if err := ensureLocksGitignored(repo); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Started worktree for %s at %s\n", ticketID, path)
		return nil
	},
}

var worktreeStopCmd = &cobra.Command{
	Use:   "stop <TKT-NNN>",
	Short: "Release lock for a ticket worktree",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := releaseLockForTicket(repo, args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Released worktree lock for %s\n", args[0])
		return nil
	},
}

func init() {
	worktreeStartCmd.Flags().BoolVar(&worktreeForce, "force", false, "allow worktree start even when relation checks warn")
	worktreeCmd.AddCommand(worktreeStartCmd)
	worktreeCmd.AddCommand(worktreeStopCmd)
	rootCmd.AddCommand(worktreeCmd)
}
