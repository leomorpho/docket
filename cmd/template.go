package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage AC templates",
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available AC templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, name := range listTemplates(repo) {
			fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		return nil
	},
}

var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show AC items for a template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		items, ok := getTemplate(repo, args[0])
		if !ok {
			return fmt.Errorf("template %s not found", args[0])
		}
		fmt.Fprintln(cmd.OutOrStdout(), formatTemplate(args[0], items))
		return nil
	},
}

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	rootCmd.AddCommand(templateCmd)
}
