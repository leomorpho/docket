package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Inspect and manage ticket file locks",
}

var (
	lockReleaseTicket string
	lockReleaseYes    bool
)

var lockStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active file claims by ticket",
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := refreshLockClaims(repo)
		if err != nil {
			return err
		}
		if len(st.Locks) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No active locks.")
			return nil
		}
		for _, l := range st.Locks {
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", l.TicketID, l.WorktreePath)
			for _, f := range l.Files {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", f)
			}
		}
		return nil
	},
}

var lockReleaseCmd = &cobra.Command{
	Use:   "release <TKT-NNN>",
	Short: "Release lock for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requirePrivilegedSurface(cmd, lockReleaseTicket, "release lock "+args[0], lockReleaseYes); err != nil {
			return err
		}
		if err := releaseLockForTicket(repo, args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Released lock for %s\n", args[0])
		return nil
	},
}

func init() {
	lockReleaseCmd.Flags().StringVar(&lockReleaseTicket, "ticket", "", "ticket ID authorizing this privileged lock release")
	lockReleaseCmd.Flags().BoolVar(&lockReleaseYes, "yes", false, "skip interactive confirmation prompt")
	lockCmd.AddCommand(lockStatusCmd)
	lockCmd.AddCommand(lockReleaseCmd)
	rootCmd.AddCommand(lockCmd)
}
