package validate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/claim"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/vcs"
	"github.com/leomorpho/docket/internal/workflow"
)

func TestServiceValidateAcceptsDoneWhenCommitExistsWorktreeCleanAndACPasses(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)
	sha := commitWorktreeChange(t, env.worktreePath, "feature.txt", "ok\n")

	result, err := env.service.Validate(context.Background(), agentrun.ValidationInput{
		TicketID:     "TKT-377",
		RepoRoot:     env.repoRoot,
		WorktreePath: env.worktreePath,
		Branch:       "docket/TKT-377",
		Result: agentrun.Result{
			Status:    agentrun.StatusDone,
			TicketID:  "TKT-377",
			Role:      agentrun.RoleImplementer,
			CommitSHA: sha,
			Tests:     "passed",
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected validation accepted, got %#v", result)
	}
}

func TestServiceValidateRejectsMissingCommitAndDirtyWorktree(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)
	if err := os.WriteFile(filepath.Join(env.worktreePath, "feature.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	result, err := env.service.Validate(context.Background(), agentrun.ValidationInput{
		TicketID:     "TKT-377",
		RepoRoot:     env.repoRoot,
		WorktreePath: env.worktreePath,
		Branch:       "docket/TKT-377",
		Result: agentrun.Result{
			Status:    agentrun.StatusDone,
			TicketID:  "TKT-377",
			Role:      agentrun.RoleImplementer,
			CommitSHA: "deadbeef",
			Tests:     "passed",
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Accepted {
		t.Fatalf("expected validation rejection, got %#v", result)
	}
	joined := strings.Join(result.Reasons, "\n")
	for _, want := range []string{"does not exist", "worktree is not clean"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("validation reasons missing %q in %q", want, joined)
		}
	}
}

func TestServiceFinalizeAdvancesDoneRunToReviewAndRecordsHandoff(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)
	sha := commitWorktreeChange(t, env.worktreePath, "feature.txt", "ok\n")

	result, err := env.service.Finalize(context.Background(), agentrun.ValidationInput{
		TicketID:     "TKT-377",
		RepoRoot:     env.repoRoot,
		WorktreePath: env.worktreePath,
		Branch:       "docket/TKT-377",
		Result: agentrun.Result{
			Status:    agentrun.StatusDone,
			TicketID:  "TKT-377",
			Role:      agentrun.RoleImplementer,
			CommitSHA: sha,
			Tests:     "passed",
		},
	})
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if !result.Accepted {
		t.Fatalf("expected accepted finalize result, got %#v", result)
	}

	tkt, err := env.store.GetTicket(context.Background(), "TKT-377")
	if err != nil {
		t.Fatalf("GetTicket() error = %v", err)
	}
	if tkt.State != ticket.State("in-review") {
		t.Fatalf("ticket state = %q, want in-review", tkt.State)
	}
	if !strings.Contains(strings.Join(tkt.LinkedCommits, ","), sha) {
		t.Fatalf("expected linked commit %s in %#v", sha, tkt.LinkedCommits)
	}
	for _, want := range []string{"**Current state:**", "**Decisions made:**", "**Files touched:**", "**Remaining work:**", "**AC status:**"} {
		if !strings.Contains(tkt.Handoff, want) {
			t.Fatalf("handoff missing %q in %q", want, tkt.Handoff)
		}
	}
	if len(tkt.Comments) == 0 {
		t.Fatalf("expected outcome comment recorded")
	}
}

func TestServiceFinalizeRecordsStuckRunWithoutAdvancingTicket(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)

	result, err := env.service.Finalize(context.Background(), agentrun.ValidationInput{
		TicketID:     "TKT-377",
		RepoRoot:     env.repoRoot,
		WorktreePath: env.worktreePath,
		Branch:       "docket/TKT-377",
		Result: agentrun.Result{
			Status:   agentrun.StatusStuck,
			TicketID: "TKT-377",
			Role:     agentrun.RoleImplementer,
			Reason:   "baseline tests failing",
		},
	})
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if result.Accepted {
		t.Fatalf("stuck finalize should not be accepted: %#v", result)
	}

	tkt, err := env.store.GetTicket(context.Background(), "TKT-377")
	if err != nil {
		t.Fatalf("GetTicket() error = %v", err)
	}
	if tkt.State != ticket.State("in-progress") {
		t.Fatalf("ticket state = %q, want in-progress", tkt.State)
	}
	if len(tkt.Comments) == 0 || !strings.Contains(tkt.Comments[len(tkt.Comments)-1].Body, "baseline tests failing") {
		t.Fatalf("expected stuck comment recorded, got %#v", tkt.Comments)
	}
}

type validationEnv struct {
	repoRoot     string
	worktreePath string
	store        *local.Store
	service      *Service
}

func buildValidationEnv(t *testing.T) validationEnv {
	t.Helper()

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	runGit(t, repoRoot, "config", "user.email", "test@example.com")
	runGit(t, repoRoot, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGit(t, repoRoot, "add", ".")
	runGit(t, repoRoot, "commit", "-m", "seed")

	cfg := ticket.DefaultConfig()
	if err := ticket.SaveConfig(repoRoot, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	store := local.New(repoRoot)
	if err := store.CreateTicket(context.Background(), &ticket.Ticket{
		ID:          "TKT-377",
		Seq:         377,
		Title:       "Validator",
		State:       ticket.State("todo"),
		Priority:    1,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		CreatedBy:   "human:test",
		Description: "Validator ticket",
		AC: []ticket.AcceptanceCriterion{
			{Description: "feature file exists", Run: "test -f feature.txt"},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	flow := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot))
	if _, worktreePath, err := flow.StartTask(context.Background(), "TKT-377", "agent:test", cfg); err != nil {
		t.Fatalf("StartTask() error = %v", err)
	} else {
		store = local.New(repoRoot)
		service := New(Dependencies{
			RepoRoot: repoRoot,
			Store:    store,
			Workflow: flow,
		})
		return validationEnv{
			repoRoot:     repoRoot,
			worktreePath: worktreePath,
			store:        store,
			service:      service,
		}
	}
	return validationEnv{}
}

func commitWorktreeChange(t *testing.T, worktreePath, relPath, contents string) string {
	t.Helper()
	path := filepath.Join(worktreePath, relPath)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}
	runGit(t, worktreePath, "add", ".")
	runGit(t, worktreePath, "commit", "-m", "feat: update\n\nTicket: TKT-377")
	return strings.TrimSpace(runGitOutput(t, worktreePath, "rev-parse", "HEAD"))
}

func runGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
