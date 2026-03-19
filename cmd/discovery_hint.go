package cmd

import (
	"fmt"
	"io"
)

func shouldEmitDiscoveryHint(outputFormat string) bool {
	switch outputFormat {
	case "json", "md":
		return false
	default:
		return true
	}
}

func discoveryHintLine() string {
	return "Hint: Use `docket skill list --format json` to discover built-in skills, `docket skill invoke <skill-id>` when one matches the task, `docket ls --full` for the full graph, `docket search \"query\"` for ticket discovery, `docket start --format json` for workflow guidance, and `docket capabilities --format json` for capability discovery."
}

func printDiscoveryHint(out io.Writer, outputFormat string) {
	if !shouldEmitDiscoveryHint(outputFormat) {
		return
	}
	// Keep this hint on common entry points even when repetitive; agents frequently
	// enter through list/show and miss one-time startup guidance.
	fmt.Fprintln(out)
	fmt.Fprintln(out, discoveryHintLine())
	fmt.Fprintln(out)
}
