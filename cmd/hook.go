package cmd

import (
	"fmt"
	"strings"

	"github.com/leomorpho/docket/internal/capabilities"
	"github.com/spf13/cobra"
)

type hookEventView struct {
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Blocking bool   `json:"blocking"`
}

type hookStatusView struct {
	Namespace  string          `json:"namespace"`
	Invocation string          `json:"invocation"`
	Execution  string          `json:"execution"`
	Ready      bool            `json:"ready"`
	Readiness  string          `json:"readiness"`
	Events     []hookEventView `json:"events"`
}

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Inspect hook metadata and status (introspection only)",
}

var hookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List known hook events and modes",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := buildHookStatusView(repo)
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, map[string]any{
				"total":  len(status.Events),
				"events": status.Events,
			})
			return nil
		}
		if len(status.Events) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No hook events defined.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Hook events (%d):\n", len(status.Events))
		for _, event := range status.Events {
			kind := "advisory"
			if event.Blocking {
				kind = "blocking"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s)\n", event.Name, kind)
		}
		return nil
	},
}

var hookShowCmd = &cobra.Command{
	Use:   "show <event>",
	Short: "Show details for one hook event",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := buildHookStatusView(repo)
		if err != nil {
			return err
		}
		target := strings.ToLower(strings.TrimSpace(args[0]))
		for _, event := range status.Events {
			if strings.ToLower(event.Name) != target {
				continue
			}
			if format == "json" {
				printJSON(cmd, map[string]any{
					"event":      event,
					"namespace":  status.Namespace,
					"invocation": status.Invocation,
					"execution":  status.Execution,
				})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Hook event: %s\n", event.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Mode: %s\n", event.Mode)
			fmt.Fprintf(cmd.OutOrStdout(), "Namespace: %s\n", status.Namespace)
			fmt.Fprintf(cmd.OutOrStdout(), "Invocation: %s\n", status.Invocation)
			fmt.Fprintf(cmd.OutOrStdout(), "Execution: %s\n", status.Execution)
			return nil
		}
		return fmt.Errorf("hook event %s not found", strings.TrimSpace(args[0]))
	},
}

var hookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show hook readiness and event metadata",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := buildHookStatusView(repo)
		if err != nil {
			return err
		}
		if format == "json" {
			printJSON(cmd, status)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Hook readiness: %s\n", status.Readiness)
		fmt.Fprintf(cmd.OutOrStdout(), "Namespace: %s\n", status.Namespace)
		fmt.Fprintf(cmd.OutOrStdout(), "Invocation: %s\n", status.Invocation)
		fmt.Fprintf(cmd.OutOrStdout(), "Execution: %s\n", status.Execution)
		if len(status.Events) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Events:")
			for _, event := range status.Events {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s)\n", event.Name, event.Mode)
			}
		}
		if !status.Ready {
			fmt.Fprintln(cmd.OutOrStdout(), "Remediation: run `docket bootstrap` to repair hook wiring.")
		}
		return nil
	},
}

func buildHookStatusView(repoRoot string) (hookStatusView, error) {
	runtime, _, err := capabilities.EnsureRuntimeContract(repoRoot)
	if err != nil {
		return hookStatusView{}, err
	}
	hookStale, _, err := artifactStatus(repoRoot)
	if err != nil {
		hookStale = true
	}
	events := make([]hookEventView, 0, len(runtime.Hooks.Events))
	for _, event := range runtime.Hooks.Events {
		events = append(events, hookEventView{
			Name:     event.Name,
			Mode:     event.Mode,
			Blocking: event.Blocking,
		})
	}
	ready := !hookStale
	readiness := "needs-setup"
	if ready {
		readiness = "ready"
	}
	return hookStatusView{
		Namespace:  runtime.Hooks.Namespace,
		Invocation: runtime.Hooks.Invocation,
		Execution:  runtime.Hooks.Execution,
		Ready:      ready,
		Readiness:  readiness,
		Events:     events,
	}, nil
}

func init() {
	hookCmd.AddCommand(hookListCmd)
	hookCmd.AddCommand(hookShowCmd)
	hookCmd.AddCommand(hookStatusCmd)
	rootCmd.AddCommand(hookCmd)
}
