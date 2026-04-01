package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var mergeSubjectTicketRE = regexp.MustCompile(`^Merge ticket branch docket/(TKT-[0-9]+)(?:$|[^0-9].*)`)

var hookPostMergeReviewSyncCmd = &cobra.Command{
	Use:    "__hook-post-merge-review-sync",
	Short:  "legacy post-merge hook placeholder",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookPostMergeReviewSync(context.Background(), repo, cmd.ErrOrStderr())
	},
}

func runHookPostMergeReviewSync(ctx context.Context, repoRoot string, sink interface{ Write([]byte) (int, error) }) error {
	_ = ctx
	ticketID, err := mergedTicketFromHead(repoRoot)
	if err != nil {
		return err
	}
	if ticketID == "" {
		return nil
	}
	if sink != nil {
		_, _ = sink.Write([]byte(fmt.Sprintf("docket: post-merge review sync is disabled for %s; managed runs now finalize directly to validated/completed\n", ticketID)))
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
