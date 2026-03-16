package cmd

import "github.com/spf13/cobra"

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Manage backlog-oriented workflows",
}

func init() {
	rootCmd.AddCommand(backlogCmd)
}
