package cmd

import "github.com/spf13/cobra"

var semanticCmd = &cobra.Command{
	Use:   "semantic",
	Short: "Manage local semantic indexing features",
}

func init() {
	rootCmd.AddCommand(semanticCmd)
}
