package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	repo   string
	format string
)

var rootCmd = &cobra.Command{
	Use:   "docket",
	Short: "git-native ticket system for AI-assisted development",
	Long:  `A git-native ticket system built for human + LLM agentic workflows.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
		os.Exit(1)
	}

	rootCmd.PersistentFlags().StringVar(&repo, "repo", cwd, "path to repo root")
	rootCmd.PersistentFlags().StringVar(&format, "format", "human", "output format (human or json)")
}
