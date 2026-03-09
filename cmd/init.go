package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leoaudibert/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize docket in a repository",
	Run: func(cmd *cobra.Command, args []string) {
		cfgPath := ticket.ConfigPath(repo)
		if _, err := os.Stat(cfgPath); err == nil {
			if format == "json" {
				printJSON(cmd, map[string]string{"status": "already initialized", "path": filepath.Dir(cfgPath)})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "docket already initialized in %s\n", filepath.Dir(cfgPath))
			}
			return
		}

		// 1. Create directories
		docketDir := filepath.Dir(cfgPath)
		ticketsDir := filepath.Join(docketDir, "tickets")
		if err := os.MkdirAll(ticketsDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
			os.Exit(2)
		}

		// 2. Write config
		cfg := ticket.DefaultConfig()
		if err := ticket.SaveConfig(repo, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(2)
		}

		// 3. Update gitignore
		gitignorePath := filepath.Join(repo, ".gitignore")
		ignoreContent := "\n# docket\n.docket/index.db\n.docket/tickets/*/sessions/\n"

		data, err := os.ReadFile(gitignorePath)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error reading .gitignore: %v\n", err)
			os.Exit(2)
		}

		if !strings.Contains(string(data), ".docket/index.db") {
			f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error opening .gitignore: %v\n", err)
				os.Exit(2)
			}
			if _, err := f.WriteString(ignoreContent); err != nil {
				f.Close()
				fmt.Fprintf(os.Stderr, "Error writing to .gitignore: %v\n", err)
				os.Exit(2)
			}
			f.Close()
		}

		// 4. Output
		if format == "json" {
			printJSON(cmd, map[string]string{"status": "ok", "path": docketDir})
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized docket in %s/\n\n", docketDir)
			fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
			fmt.Fprintln(cmd.OutOrStdout(), "  docket create --title \"My first ticket\"")
			fmt.Fprintln(cmd.OutOrStdout(), "  docket board")
		}
	},
}

func printJSON(cmd *cobra.Command, v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
}

func init() {
	rootCmd.AddCommand(initCmd)
}
