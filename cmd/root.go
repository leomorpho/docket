package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/leomorpho/docket/internal/artifacts"
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
		if err := ensureDocketHome(); err != nil {
			return err
		}
		prepareVersionNotice(cmd)
		repoRoot := ticketRepoRoot(repo)

		// Tamper detection is non-blocking but runs on command startup.
		if _, err := os.Stat(artifacts.RepoPath(repoRoot, artifacts.RepoConfigJSON)); err != nil {
			return nil
		}
		s := local.New(repoRoot)
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
		printGlobalSkillHint(cmd, cmd.OutOrStdout(), format)
		flushVersionNotice(cmd)
		// The CLI process handles one command per invocation; reset globals for in-process tests.
		automationMode = false
		runDisableReview = false
		runInactivityLimit = 0
		runManagedAdapter = ""
		runWatch = false
		if f := cmd.Root().PersistentFlags().Lookup("automation"); f != nil {
			f.Changed = false
		}
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
	rootCmd.PersistentFlags().BoolVar(&automationMode, "automation", false, "enable non-interactive deterministic automation mode")
}
