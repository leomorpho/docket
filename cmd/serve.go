package cmd

import (
	"fmt"
	"os"

	"github.com/leomorpho/docket/internal/mcp"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var serveMCP bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run docket server modes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !serveMCP {
			return fmt.Errorf("currently only --mcp is supported")
		}
		cfg, err := ticket.LoadConfig(repo)
		if err != nil {
			return err
		}
		deps := newRuntimeDeps(repo)
		mcpDeps := mcp.NewDispatchDeps(repo, deps.store, deps.workflow, deps.claimer, cfg)
		return mcp.ServeMCPWithDeps(os.Stdin, cmd.OutOrStdout(), mcpDeps)
	},
}

func init() {
	serveCmd.Flags().BoolVar(&serveMCP, "mcp", false, "serve MCP protocol over stdin/stdout")
	rootCmd.AddCommand(serveCmd)
}
