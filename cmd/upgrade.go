package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var upgradeCheck bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade docket-managed artifacts and run migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			upgradeCheck = false
			if f := cmd.Flags().Lookup("check"); f != nil {
				f.Changed = false
			}
		}()

		gitDir := filepath.Join(repo, ".git")
		if stat, err := os.Stat(gitDir); err != nil || !stat.IsDir() {
			return fmt.Errorf("git repository not detected at %s", gitDir)
		}

		hookStale, claudeStale, err := artifactStatus(repo)
		if err != nil {
			return err
		}
		manifest, manifestErr := loadInstallManifest(repo)
		manifestStale := manifestErr != nil

		if upgradeCheck {
			if hookStale || claudeStale || manifestStale {
				return fmt.Errorf("docket artifacts are stale; run `docket upgrade`")
			}
			if format != "json" {
				fmt.Fprintln(cmd.OutOrStdout(), "docket artifacts are up to date.")
			} else {
				printJSON(cmd, map[string]any{"stale": false})
			}
			return nil
		}

		fromVersion := normalizeVersion(Version)
		if !manifestStale && manifest.DocketVersion != "" {
			fromVersion = manifest.DocketVersion
		}
		if err := runMigrations(repo, fromVersion); err != nil {
			return err
		}
		if _, err := writeHook(repo); err != nil {
			return err
		}
		if _, err := ensureClaudeManagedBlock(repo); err != nil {
			return err
		}
		if err := ensureConfigYAML(repo); err != nil {
			return err
		}
		if err := writeInstallManifest(repo); err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]any{
				"upgraded": true,
			})
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Upgraded docket-managed artifacts.")
		return nil
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "check whether managed artifacts are stale")
	rootCmd.AddCommand(upgradeCmd)
}
