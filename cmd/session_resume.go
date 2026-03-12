package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var sessionResumeCmd = &cobra.Command{
	Use:   "resume <TKT-NNN>",
	Short: "Print structured checkpoint context for agent resume",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		paths, err := listCheckpointPaths(repo, id)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("no checkpoints found for %s", id)
		}
		latest := paths[len(paths)-1]
		data, err := os.ReadFile(latest)
		if err != nil {
			return err
		}
		var cp checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "RESUME_CONTEXT\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ticket=%s\n", cp.TicketID)
		fmt.Fprintf(cmd.OutOrStdout(), "created_at=%s\n", cp.CreatedAt)
		fmt.Fprintf(cmd.OutOrStdout(), "ac=%d/%d\n", cp.ACDone, cp.ACTotal)
		fmt.Fprintf(cmd.OutOrStdout(), "branch=%s\n", cp.Branch)
		fmt.Fprintf(cmd.OutOrStdout(), "worktree=%s\n", cp.WorktreePath)
		fmt.Fprintf(cmd.OutOrStdout(), "changed_files=[%s]\n", strings.Join(cp.ChangedFiles, ", "))
		fmt.Fprintf(cmd.OutOrStdout(), "last_comments=[%s]\n", strings.Join(cp.LastComments, " | "))
		if strings.TrimSpace(cp.Summary) != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "summary=%s\n", strings.TrimSpace(cp.Summary))
		}
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionResumeCmd)
}
