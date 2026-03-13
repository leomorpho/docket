package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize docket in a repository",
	Long: `Initialize docket in a repository.

Secure features require DOCKET_HOME to be set to a writable directory outside the repo,
for example: DOCKET_HOME=$HOME/.docket-home`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := ticket.ConfigPath(repo)
		if _, err := os.Stat(cfgPath); err == nil {
			if format == "json" {
				printJSON(cmd, map[string]string{"status": "already initialized", "path": filepath.Dir(cfgPath)})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "docket already initialized in %s\n", filepath.Dir(cfgPath))
			}
			return nil
		}

		// 1. Create directories
		docketDir := filepath.Dir(cfgPath)
		ticketsDir := filepath.Join(docketDir, "tickets")
		if err := os.MkdirAll(ticketsDir, 0755); err != nil {
			return fmt.Errorf("creating directories: %w", err)
		}

		// 2. Write config
		cfg := ticket.DefaultConfig()
		if err := ticket.SaveConfig(repo, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		// 3. Update gitignore
		gitignorePath := filepath.Join(repo, ".gitignore")
		ignoreContent := "\n# docket\n.docket/index.db\n.docket/tickets/*/sessions/\n"

		data, err := os.ReadFile(gitignorePath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading .gitignore: %w", err)
		}

		if !strings.Contains(string(data), ".docket/index.db") {
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("opening .gitignore: %w", err)
			}
			if _, err := f.WriteString(ignoreContent); err != nil {
				f.Close()
				return fmt.Errorf("writing to .gitignore: %w", err)
			}
			f.Close()
		}

		// 4. Output
		if format == "json" {
			printJSON(cmd, map[string]string{"status": "ok", "path": docketDir})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized docket in %s/\n\n", docketDir)
			fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
			fmt.Fprintln(cmd.OutOrStdout(), "  export DOCKET_HOME=$HOME/.docket-home")
			fmt.Fprintln(cmd.OutOrStdout(), "  docket create --title \"My first ticket\"")
			fmt.Fprintln(cmd.OutOrStdout(), "  docket board")
		}
		return nil
	},
}

func printJSON(cmd *cobra.Command, v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
}

func init() {
	rootCmd.AddCommand(initCmd)
}
