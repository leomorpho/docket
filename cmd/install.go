package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install docket-managed git hook and CLAUDE.md instructions",
	RunE: func(cmd *cobra.Command, args []string) error {
		gitDir := filepath.Join(repo, ".git")
		if stat, err := os.Stat(gitDir); err != nil || !stat.IsDir() {
			return fmt.Errorf("git repository not detected at %s", gitDir)
		}

		hookChanged, err := writeHook(repo)
		if err != nil {
			return err
		}
		claudeChanged, err := ensureClaudeManagedBlock(repo)
		if err != nil {
			return err
		}
		if err := writeInstallManifest(repo); err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"hook_changed":   hookChanged,
				"claude_changed": claudeChanged,
				"manifest_path":  installManifestPath(repo),
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Installed docket artifacts.\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  hook: %s\n", preCommitHookPath(repo))
		fmt.Fprintf(cmd.OutOrStdout(), "  manifest: %s\n", installManifestPath(repo))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
