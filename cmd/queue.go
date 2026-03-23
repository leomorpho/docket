package cmd

import "github.com/spf13/cobra"

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Inspect and repair workable queue health",
}

func init() {
	rootCmd.AddCommand(queueCmd)
}
