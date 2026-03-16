package cmd

import "github.com/spf13/cobra"

var ticketCmd = &cobra.Command{
	Use:   "ticket",
	Short: "Manage ticket-oriented workflows",
}

func init() {
	rootCmd.AddCommand(ticketCmd)
}
