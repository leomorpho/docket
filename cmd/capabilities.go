package cmd

import (
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/adapters"
	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/spf13/cobra"
)

type capabilitiesView struct {
	Adapter       adapters.Metadata            `json:"adapter"`
	AdapterSource string                       `json:"adapter_source,omitempty"`
	Contract      capabilities.RuntimeContract `json:"contract"`
}

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Render runtime capabilities contract in markdown or JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		contract, err := capabilities.LoadRuntimeContract(repo)
		if err != nil {
			return fmt.Errorf("load capabilities contract: %w", err)
		}

		resolution := adapters.DefaultRegistry().ResolveWithInfo(repo, "")
		view := capabilitiesView{
			Adapter:       resolution.Adapter.Metadata(),
			AdapterSource: resolution.Source,
			Contract:      contract,
		}
		if format == "json" {
			printJSON(cmd, view)
			return nil
		}

		fmt.Fprintln(cmd.OutOrStdout(), renderCapabilitiesMarkdown(view))
		return nil
	},
}

func renderCapabilitiesMarkdown(view capabilitiesView) string {
	lines := []string{
		"# Docket Capabilities",
		"",
		fmt.Sprintf("- Adapter: `%s` (%s)", view.Adapter.ID, view.Adapter.DisplayName),
		fmt.Sprintf("- Adapter source: `%s`", view.AdapterSource),
		fmt.Sprintf("- Contract version: `%d`", view.Contract.Version),
		fmt.Sprintf("- Contract hash: `%s`", view.Contract.Hash),
		"",
		"## Workflow Phases",
	}
	for _, phase := range view.Contract.Workflow.Phases {
		lines = append(lines, fmt.Sprintf("- `%s`", phase))
	}
	lines = append(lines, "", "## Hook Events")
	for _, event := range view.Contract.Hooks.Events {
		mode := "non-blocking"
		if event.Blocking {
			mode = "blocking"
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s)", event.Name, mode))
	}
	lines = append(lines, "", "## Skills")
	for _, skill := range view.Contract.Skills.Inventory {
		optional := "required"
		if skill.Optional {
			optional = "optional"
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s)", skill.Name, optional))
	}
	if strings.TrimSpace(view.Contract.Compatibility.UpgradeNotes) != "" {
		lines = append(lines, "", "## Compatibility", view.Contract.Compatibility.UpgradeNotes)
	}
	return strings.Join(lines, "\n")
}

func init() {
	rootCmd.AddCommand(capabilitiesCmd)
}
