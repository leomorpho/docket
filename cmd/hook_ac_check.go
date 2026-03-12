package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leoaudibert/docket/internal/store/local"
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

		reader := bufio.NewReader(os.Stdin)
		updated := false
		for i := range t.AC {
			ac := &t.AC[i]
			if ac.Done {
				continue
			}
			if strings.TrimSpace(ac.Run) != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "docket: running AC %d command for %s: %s\n", i+1, id, ac.Run)
				exitCode, evidence := runACCommand(ac.Run)
				if exitCode != 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "docket: AC %d failed for %s\n%s\n", i+1, id, evidence)
					return fmt.Errorf("acceptance criteria check failed for %s", id)
				}
				ac.Done = true
				ac.Evidence = evidence
				updated = true
				fmt.Fprintf(cmd.ErrOrStderr(), "docket: AC %d passed for %s\n", i+1, id)
				continue
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "docket: Did you complete AC %d for %s: %q? [y/N]: ", i+1, id, ac.Description)
			answer, _ := reader.ReadString('\n')
			normalized := strings.ToLower(strings.TrimSpace(answer))
			if normalized != "y" && normalized != "yes" {
				return fmt.Errorf("acceptance criteria check failed for %s (use `docket ac complete %s --step %d --evidence \"...\"`)", id, id, i+1)
			}
			ac.Done = true
			ac.Evidence = "confirmed in pre-commit prompt"
			updated = true
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
