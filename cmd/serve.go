package cmd

import (
	"fmt"
	"os"

	"github.com/leoaudibert/docket/internal/mcp"
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
		return mcp.ServeMCP(os.Stdin, cmd.OutOrStdout(), repo)
	},
}

func init() {
	serveCmd.Flags().BoolVar(&serveMCP, "mcp", false, "serve MCP protocol over stdin/stdout")
	rootCmd.AddCommand(serveCmd)
}
