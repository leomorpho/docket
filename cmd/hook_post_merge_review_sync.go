package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/spf13/cobra"
)

var mergeSubjectTicketRE = regexp.MustCompile(`^Merge ticket branch docket/(TKT-[0-9]+)(?:$|[^0-9].*)`)

var hookPostMergeReviewSyncCmd = &cobra.Command{
	Use:    "__hook-post-merge-review-sync",
	Short:  "internal post-merge review state synchronizer",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookPostMergeReviewSync(context.Background(), repo, cmd.ErrOrStderr())
	},
}

func runHookPostMergeReviewSync(ctx context.Context, repoRoot string, sink interface{ Write([]byte) (int, error) }) error {
	ticketID, err := mergedTicketFromHead(repoRoot)
	if err != nil {
		return err
	}
	if ticketID == "" {
		return nil
	}

	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return err
	}
	s := local.New(repoRoot)
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		return err
	}
	if !cfg.StateHasRole(string(t.State), "active") {
		return nil
	}

	reviewState := ticket.State(nextStateForRole(cfg, t.State, "review", "in-review"))
	if err := ticket.ValidateTransition(cfg, t.State, reviewState); err != nil {
		return nil
	}

	fromState := t.State
	t.State = reviewState
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTicket(ctx, t); err != nil {
		return err
	}
	_ = releaseLockForTicket(repoRoot, t.ID)

	if sink != nil {
		_, _ = sink.Write([]byte(fmt.Sprintf("docket: auto-transitioned %s from %s to %s after merge\n", t.ID, fromState, t.State)))
	}
	return nil
}

func mergedTicketFromHead(repoRoot string) (string, error) {
	parentsRaw, err := runGitRead(repoRoot, "show", "-s", "--format=%P", "HEAD")
	if err != nil {
		return "", err
	}
	if len(strings.Fields(parentsRaw)) < 2 {
		return "", nil
	}

	subject, err := runGitRead(repoRoot, "show", "-s", "--format=%s", "HEAD")
	if err != nil {
		return "", err
	}
	subject = strings.TrimSpace(subject)
	if m := mergeSubjectTicketRE.FindStringSubmatch(subject); len(m) > 1 {
		return m[1], nil
	}
	return "", nil
}

func runGitRead(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func init() {
	rootCmd.AddCommand(hookPostMergeReviewSyncCmd)
}
