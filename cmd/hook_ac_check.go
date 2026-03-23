package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var hookACCheckCmd = &cobra.Command{
	Use:    "__hook-ac-check <TKT-NNN>",
	Short:  "internal pre-commit AC checker",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookACCheck(cmd, args[0])
	},
}

func runHookACCheck(cmd *cobra.Command, id string) (runErr error) {
	recorder := lifecycleStart(cmd.ErrOrStderr(), "__hook-ac-check", id, detectActor())
	runStatus := lifecycle.StatusOK
	defer func() {
		lifecycleRunEnd(cmd.ErrOrStderr(), recorder, runStatus)
	}()

	if os.Getenv("DOCKET_SKIP_AC") == "1" {
		lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusOK)
		return nil
	}

	s := local.New(repo)
	t, err := s.GetTicket(context.Background(), id)
	if err != nil {
		runStatus = lifecycle.StatusFailed
		lifecycleToolFailure(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, "store.get_ticket", err)
		lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusFailed)
		return err
	}
	if t == nil {
		lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusOK)
		return nil
	}

	updated := false
	enforce := shouldEnforceSecurityHooks(repo)
	for i := range t.AC {
		ac := &t.AC[i]
		if ac.Done {
			continue
		}
		if strings.TrimSpace(ac.Run) != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "docket: running AC %d command for %s: %s\n", i+1, id, ac.Run)
			exitCode, evidence := runACCommand(ac.Run)
			if exitCode != 0 {
				failure := fmt.Errorf("exit_code=%d: %s", exitCode, strings.TrimSpace(evidence))
				runStatus = lifecycle.StatusFailed
				lifecycleToolFailure(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, "ac.run", failure)
				lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusFailed)

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
			err := fmt.Errorf("acceptance criteria check failed for %s (use `docket ac complete %s --step %d --evidence \"...\"`)", id, id, i+1)
			runStatus = lifecycle.StatusFailed
			lifecycleToolFailure(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, "ac.manual", err)
			lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusFailed)
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "docket: advisory: AC %d incomplete for %s (%q). Run `docket ac complete %s --step %d --evidence \"...\"`.\n", i+1, id, ac.Description, id, i+1)
		lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusOK)
		return nil
	}

	if updated {
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := s.UpdateTicket(context.Background(), t); err != nil {
			runStatus = lifecycle.StatusFailed
			lifecycleToolFailure(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, "store.update_ticket", err)
			lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusFailed)
			return err
		}
	}
	lifecyclePhaseEnd(cmd.ErrOrStderr(), recorder, lifecyclePhaseACCheck, lifecycle.StatusOK)
	return nil
}

func shouldEnforceSecurityHooks(repoRoot string) bool {
	if os.Getenv("DOCKET_HOOK_AC_ENFORCE") == "1" || os.Getenv("DOCKET_ENFORCE_HOOKS") == "1" {
		return true
	}
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.SecurityEnforcement
}

func init() {
	rootCmd.AddCommand(hookACCheckCmd)
}
