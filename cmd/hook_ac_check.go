package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/spf13/cobra"
)

var hookACCheckCmd = &cobra.Command{
	Use:    "__hook-ac-check <TKT-NNN>",
	Short:  "internal pre-commit AC checker",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("DOCKET_SKIP_AC") == "1" {
			return nil
		}

		id := args[0]
		s := local.New(repo)
		t, err := s.GetTicket(context.Background(), id)
		if err != nil {
			return err
		}
		if t == nil {
			return nil
		}

		updated := false
		enforce := os.Getenv("DOCKET_HOOK_AC_ENFORCE") == "1" || os.Getenv("DOCKET_ENFORCE_HOOKS") == "1"
		for i := range t.AC {
			ac := &t.AC[i]
			if ac.Done {
				continue
			}
			if strings.TrimSpace(ac.Run) != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "docket: running AC %d command for %s: %s\n", i+1, id, ac.Run)
				exitCode, evidence := runACCommand(ac.Run)
				if exitCode != 0 {
					msg := fmt.Sprintf("docket: AC %d failed for %s\n%s\n", i+1, id, evidence)
					if enforce {
						fmt.Fprint(cmd.ErrOrStderr(), msg)
						return fmt.Errorf("acceptance criteria check failed for %s", id)
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "docket: advisory: %sRun `docket ac complete %s --step %d --evidence \"...\"` after remediation.\n", strings.TrimSpace(msg)+"\n", id, i+1)
					return nil
				}
				ac.Done = true
				ac.Evidence = evidence
				updated = true
				fmt.Fprintf(cmd.ErrOrStderr(), "docket: AC %d passed for %s\n", i+1, id)
				continue
			}

			if enforce {
				return fmt.Errorf("acceptance criteria check failed for %s (use `docket ac complete %s --step %d --evidence \"...\"`)", id, id, i+1)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "docket: advisory: AC %d incomplete for %s (%q). Run `docket ac complete %s --step %d --evidence \"...\"`.\n", i+1, id, ac.Description, id, i+1)
			return nil
		}

		if updated {
			t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
			if err := s.UpdateTicket(context.Background(), t); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookACCheckCmd)
}
