package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var errACIncomplete = errors.New("acceptance criteria incomplete")
var acCheckDryRun bool

var acCheckCmd = &cobra.Command{
	Use:          "check <TKT-NNN>",
	Short:        "Check whether all acceptance criteria are complete",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer func() {
			acCheckDryRun = false
			if f := cmd.Flags().Lookup("dry-run"); f != nil {
				f.Changed = false
			}
		}()

		id := args[0]
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("ticket %s not found", id)
		}

		dryRunCommands := []string{}
		for i := range t.AC {
			runCmd := strings.TrimSpace(t.AC[i].Run)
			if runCmd == "" {
				continue
			}
			dryRunCommands = append(dryRunCommands, runCmd)
			if acCheckDryRun {
				continue
			}
			exitCode, evidence := runACCommand(runCmd)
			if exitCode == 0 {
				t.AC[i].Done = true
				t.AC[i].Evidence = evidence
			} else {
				t.AC[i].Done = false
				t.AC[i].Evidence = evidence
			}
		}
		if !acCheckDryRun && len(dryRunCommands) > 0 {
			t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
			if err := s.UpdateTicket(context.Background(), t); err != nil {
				return err
			}
		}

		total := len(t.AC)
		done := 0
		var remaining []string
		for _, ac := range t.AC {
			if ac.Done {
				done++
			} else {
				remaining = append(remaining, ac.Description)
			}
		}
		complete := len(remaining) == 0

		if format == "json" {
			printJSON(cmd, map[string]interface{}{
				"ticket_id": id,
				"complete":  complete,
				"total":     total,
				"done":      done,
				"remaining": remaining,
				"dry_run":   acCheckDryRun,
				"commands":  dryRunCommands,
			})
		} else if complete {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ %s: all %d acceptance criteria met.\n", id, total)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "✗ %s: %d of %d acceptance criteria incomplete:\n", id, len(remaining), total)
			for _, r := range remaining {
				fmt.Fprintf(cmd.OutOrStdout(), "  [ ] %s\n", r)
			}
		}
		if acCheckDryRun && len(dryRunCommands) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Dry-run commands:")
			for _, command := range dryRunCommands {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", command)
			}
		}

		if !complete {
			return errACIncomplete
		}
		return nil
	},
}

func runACCommand(command string) (int, string) {
	c := exec.Command("sh", "-c", command)
	c.Dir = repo
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	err := c.Run()

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	evidence := strings.TrimSpace(strings.Join(lines, "\n"))
	if evidence == "" {
		evidence = "command ran with no output"
	}

	if err == nil {
		return 0, evidence
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), evidence
	}
	return 1, evidence
}

func init() {
	acCheckCmd.Flags().BoolVar(&acCheckDryRun, "dry-run", false, "print run commands without executing them")
	acCmd.AddCommand(acCheckCmd)
}
