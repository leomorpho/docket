package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/leomorpho/docket/internal/applyspec"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var backlogScaffoldCmd = &cobra.Command{
	Use:   "scaffold",
	Short: "Emit a schema-valid backlog apply spec template",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			cfg = ticket.DefaultConfig()
		}
		payload := map[string]any{
			"version": applyspec.SchemaVersionV1,
			"tickets": []map[string]any{
				{
					"ref":         "root",
					"title":       "Top-level work item",
					"description": "Root ticket created from scaffold template.",
					"priority":    2,
					"state":       cfg.DefaultState,
					"labels":      []string{"feature"},
					"ac":          []string{"define success criteria"},
				},
				{
					"ref":         "child",
					"title":       "Dependent child task",
					"description": "Child task that depends on root.",
					"parent_ref":  "root",
					"priority":    2,
					"state":       cfg.DefaultState,
					"labels":      []string{"feature"},
					"ac":          []string{"validate parent-child dependency"},
				},
			},
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal backlog scaffold: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	},
}

func init() {
	backlogCmd.AddCommand(backlogScaffoldCmd)
}
