package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leomorpho/docket/internal/store/local"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		prepareVersionNotice(cmd)

		// Tamper detection is non-blocking but runs on command startup.
		if _, err := os.Stat(filepath.Join(repo, ".docket", "config.json")); err != nil {
			return nil
		}
		s := local.New(repo)
		changes, err := s.DetectTamperingAll(context.Background())
		if err != nil {
			return nil
		}
		if len(changes) > 0 {
			fmt.Fprintf(os.Stderr, "warning: detected %d direct-edit changes (use `docket validate` for prescriptive fixes)\n", len(changes))
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		flushVersionNotice(cmd)
	},
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
