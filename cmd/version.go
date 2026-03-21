package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	BuildCommit = "dev"
	BuildDate   = ""
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Docket build version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		info := versionInfo{
			Version:   normalizeVersion(Version),
			Commit:    strings.TrimSpace(BuildCommit),
			BuildDate: strings.TrimSpace(BuildDate),
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		}
		if info.Commit == "" {
			info.Commit = "dev"
		}
		if format == "json" {
			printJSON(cmd, info)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "docket %s", info.Version)
		if info.Commit != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", info.Commit)
		}
		if info.BuildDate != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " built %s", info.BuildDate)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n%s %s\n", info.GoVersion, info.Platform)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
