package cmd

import "github.com/spf13/cobra"

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage conversation sessions attached to tickets",
}

func init() {
	rootCmd.AddCommand(sessionCmd)
}
