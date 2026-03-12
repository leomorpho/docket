package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/claim"
	"github.com/spf13/cobra"
)

var claimCmd = &cobra.Command{
	Use:    "claim <id>",
	Short:  "Claim a ticket (hidden)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		actor := detectActor()
		
		// If DOCKET_AGENT_ID is set, use it.
		if agentID := os.Getenv("DOCKET_AGENT_ID"); agentID != "" {
			actor = "agent:" + agentID
		}

		absRepo, err := filepath.Abs(repo)
		if err != nil {
			return err
		}
		// repo is the path to the current repo root (which might be a worktree).
		worktree := absRepo

		err = claim.Claim(repo, id, worktree, actor)
		if err != nil {
			return err
		}
		fmt.Printf("Ticket %s claimed by %s\n", id, actor)
		return nil
	},
}

var releaseCmd = &cobra.Command{
	Use:    "release <id>",
	Short:  "Release a ticket claim (hidden)",
	Args:   cobra.ExactArgs(1),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		err := claim.Release(repo, id)
		if err != nil {
			return err
		}
		fmt.Printf("Ticket %s claim released\n", id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(claimCmd)
	rootCmd.AddCommand(releaseCmd)
}
