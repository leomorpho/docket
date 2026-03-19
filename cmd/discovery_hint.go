package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func shouldEmitGlobalSkillHint(cmd *cobra.Command, outputFormat string) bool {
	switch outputFormat {
	case "json", "md", "context":
		return false
	}
	if cmd == nil || cmd == cmd.Root() || cmd.Hidden {
		return false
	}
	return true
}

func shortSkillHintLine() string {
	return "Skill hint: use `docket skill invoke <skill-id>` for built-ins; discover ids with `docket skill list --format json`."
}

func printGlobalSkillHint(cmd *cobra.Command, out io.Writer, outputFormat string) {
	if !shouldEmitGlobalSkillHint(cmd, outputFormat) {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, shortSkillHintLine())
	fmt.Fprintln(out)
}
