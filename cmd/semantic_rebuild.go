package cmd

import (
	"context"
	"fmt"

	"github.com/leomorpho/docket/internal/semantic"
	"github.com/spf13/cobra"
)

var semanticRebuildFull bool

var semanticRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the local semantic index",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg, err := loadSemanticConfig(repo)
		if err != nil {
			return err
		}
		provider, err := buildSemanticProvider(ctx, repo, cfg)
		if err != nil {
			return err
		}

		var rebuildStats semantic.RebuildStats
		if semanticRebuildFull {
			rebuildStats, err = semanticFullFn(ctx, repo, provider, cfg)
		} else {
			rebuildStats, err = semanticIncrementalFn(ctx, repo, provider, cfg)
		}
		if err != nil {
			return err
		}

		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"mode":      rebuildMode(),
				"added":     rebuildStats.Added,
				"changed":   rebuildStats.Changed,
				"deleted":   rebuildStats.Deleted,
				"unchanged": rebuildStats.Unchanged,
			})
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Semantic rebuild (%s): added=%d changed=%d deleted=%d unchanged=%d\n",
			rebuildMode(), rebuildStats.Added, rebuildStats.Changed, rebuildStats.Deleted, rebuildStats.Unchanged)
		return nil
	},
}

func rebuildMode() string {
	if semanticRebuildFull {
		return "full"
	}
	return "incremental"
}

func init() {
	semanticRebuildCmd.Flags().BoolVar(&semanticRebuildFull, "full", false, "rebuild the semantic index from scratch")
	semanticCmd.AddCommand(semanticRebuildCmd)
}
