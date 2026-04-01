package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/leomorpho/docket/internal/applyspec"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var ticketScaffoldCmd = &cobra.Command{
	Use:   "scaffold",
	Short: "Emit a schema-valid ticket apply spec template",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			cfg = ticket.DefaultConfig()
		}
		payload := map[string]any{
			"version":   applyspec.SchemaVersionV1,
			"operation": applyspec.OperationCreate,
			"ticket": map[string]any{
				"title":       "Short task title",
				"description": "Concise context and objective for this ticket.",
				"priority":    2,
				"state":       cfg.DefaultState,
				"labels":      []string{"feature"},
				"ac": []string{
					"unit tests cover expected behavior",
					"integration behavior is validated",
				},
			},
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal ticket scaffold: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	},
}

func init() {
	ticketCmd.AddCommand(ticketScaffoldCmd)
}
