package cmd

import "github.com/spf13/cobra"

var acCmd = &cobra.Command{
	Use:   "ac",
	Short: "Manage ticket acceptance criteria",
}

func init() {
	rootCmd.AddCommand(acCmd)
}
