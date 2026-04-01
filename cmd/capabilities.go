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
	QuickPath     llmQuickPath                 `json:"llm_quick_path"`
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
			QuickPath:     buildLLMQuickPath(),
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
	lines = append(lines,
		fmt.Sprintf("- Namespace: `%s`", view.Contract.Hooks.Namespace),
		fmt.Sprintf("- Invocation: `%s`", view.Contract.Hooks.Invocation),
		fmt.Sprintf("- Execution: `%s`", view.Contract.Hooks.Execution),
	)
	for _, event := range view.Contract.Hooks.Events {
		mode := event.Mode
		if strings.TrimSpace(mode) == "" {
			mode = capabilities.HookModeAdvisory
			if event.Blocking {
				mode = capabilities.HookModeEnforcement
			}
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s)", event.Name, mode))
	}
	lines = append(lines, "", "_Note: legacy secure/admin hook surfaces may still appear here as implementation details._")
	lines = append(lines, "", "## Skills")
	lines = append(lines,
		fmt.Sprintf("- Namespace: `%s`", view.Contract.Skills.Namespace),
		fmt.Sprintf("- Invocation: `%s`", view.Contract.Skills.Invocation),
	)
	for _, skill := range view.Contract.Skills.Inventory {
		optional := "required"
		if skill.Optional {
			optional = "optional"
		}
		lines = append(lines, fmt.Sprintf("- `%s` (%s)", skill.Name, optional))
		lines = append(lines, fmt.Sprintf("  - Title: %s", skill.Title))
		lines = append(lines, fmt.Sprintf("  - Intent: %s", skill.Intent))
		lines = append(lines, fmt.Sprintf("  - Command: %s", skill.Command))
		lines = append(lines, fmt.Sprintf("  - Triggers: %s", strings.Join(skill.Triggers, ", ")))
		lines = append(lines, fmt.Sprintf("  - Summary: %s", skill.Summary))
	}
	if strings.TrimSpace(view.Contract.Compatibility.UpgradeNotes) != "" {
		lines = append(lines, "", "## Compatibility", view.Contract.Compatibility.UpgradeNotes)
	}
	lines = append(lines, "", "## LLM Quick Path")
	lines = append(lines,
		"- "+view.QuickPath.Preference,
		"- `"+view.QuickPath.TicketApply+"`",
		"- `"+view.QuickPath.BacklogApply+"`",
		"- `"+view.QuickPath.ProofAttach+"`",
		"- `"+view.QuickPath.ProofVerify+"`",
		"- "+view.QuickPath.AutomationHint,
	)
	return strings.Join(lines, "\n")
}

func init() {
	rootCmd.AddCommand(capabilitiesCmd)
}
