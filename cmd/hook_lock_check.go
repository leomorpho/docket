package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var hookLockCheckCmd = &cobra.Command{
	Use:    "__hook-lock-check",
	Short:  "internal pre-commit lock overlap checker",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		st, err := refreshLockClaims(repo)
		if err != nil {
			return nil
		}
		if len(st.Locks) == 0 {
			return nil
		}

		msgPath := filepath.Join(repo, ".git", "COMMIT_EDITMSG")
		currentTickets := map[string]bool{}
		if data, err := os.ReadFile(msgPath); err == nil {
			for _, id := range parseTicketRefs(string(data)) {
				currentTickets[id] = true
			}
		}

		currentFiles := stagedFiles(repo)
		if len(currentFiles) == 0 {
			return nil
		}
		overlaps := map[string][]string{}
		for _, l := range st.Locks {
			if currentTickets[l.TicketID] {
				continue
			}
			if !activeInProgress(repo, l.TicketID) {
				continue
			}
			for _, f := range l.Files {
				if currentFiles[f] {
					overlaps[l.TicketID] = append(overlaps[l.TicketID], f)
				}
			}
		}
		if len(overlaps) == 0 {
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "docket: warning: commit files overlap with active ticket locks:")
		ids := make([]string, 0, len(overlaps))
		for id := range overlaps {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s:\n", id)
			sort.Strings(overlaps[id])
			for _, f := range overlaps[id] {
				fmt.Fprintf(cmd.ErrOrStderr(), "    - %s\n", f)
			}
		}
		return nil
	},
}

func parseTicketRefs(msg string) []string {
	re := regexpTicketRef()
	matches := re.FindAllStringSubmatch(msg, -1)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func stagedFiles(repoRoot string) map[string]bool {
	c := exec.Command("git", "-C", repoRoot, "diff", "--cached", "--name-only")
	out, err := c.Output()
	if err != nil {
		return nil
	}
	files := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files[line] = true
		}
	}
	return files
}

var ticketRefRegexp *regexp.Regexp

func regexpTicketRef() *regexp.Regexp {
	if ticketRefRegexp == nil {
		ticketRefRegexp = regexp.MustCompile(`Ticket:\s*(TKT-\d+)`)
	}
	return ticketRefRegexp
}

func init() {
	rootCmd.AddCommand(hookLockCheckCmd)
}
