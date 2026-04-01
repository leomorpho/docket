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
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
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

func TestServiceValidateRejectsMismatchedResultTicketID(t *testing.T) {
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
			TicketID:  "TKT-999",
			Role:      agentrun.RoleImplementer,
			CommitSHA: sha,
			Tests:     "passed",
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Accepted {
		t.Fatalf("expected validation rejection, got %#v", result)
	}
	if joined := strings.Join(result.Reasons, "\n"); !strings.Contains(joined, "result ticket") {
		t.Fatalf("expected mismatched ticket reason, got %q", joined)
	}
}

func TestServiceValidateRejectsCommitOutsideManagedBranch(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)
	if err := os.WriteFile(filepath.Join(env.worktreePath, "feature.txt"), []byte("off-branch\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	runGit(t, env.worktreePath, "add", ".")
	runGit(t, env.worktreePath, "commit", "-m", "feat: off branch change\n\nTicket: TKT-377")
	offBranchSHA := strings.TrimSpace(runGitOutput(t, env.worktreePath, "rev-parse", "HEAD"))
	runGit(t, env.worktreePath, "checkout", "-b", "temp/off-branch")
	runGit(t, env.worktreePath, "checkout", "docket/TKT-377")
	runGit(t, env.worktreePath, "reset", "--hard", "HEAD~1")
	if err := os.WriteFile(filepath.Join(env.worktreePath, "feature.txt"), []byte("on-branch\n"), 0o644); err != nil {
		t.Fatalf("write on-branch feature file: %v", err)
	}
	runGit(t, env.worktreePath, "add", ".")
	runGit(t, env.worktreePath, "commit", "-m", "feat: managed branch change\n\nTicket: TKT-377")

	result, err := env.service.Validate(context.Background(), agentrun.ValidationInput{
		TicketID:     "TKT-377",
		RepoRoot:     env.repoRoot,
		WorktreePath: env.worktreePath,
		Branch:       "docket/TKT-377",
		Result: agentrun.Result{
			Status:    agentrun.StatusDone,
			TicketID:  "TKT-377",
			Role:      agentrun.RoleImplementer,
			CommitSHA: offBranchSHA,
			Tests:     "passed",
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Accepted {
		t.Fatalf("expected validation rejection, got %#v", result)
	}
	if joined := strings.Join(result.Reasons, "\n"); !strings.Contains(joined, "not reachable from branch") {
		t.Fatalf("expected branch reachability reason, got %q", joined)
	}
}

func TestServiceFinalizeAdvancesDoneRunToValidatedAndRecordsHandoff(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)
	sha := commitWorktreeChange(t, env.worktreePath, "feature.txt", "ok\n")

	result, err := env.service.Finalize(context.Background(), agentrun.ValidationInput{
		TicketID:     "TKT-377",
		SessionID:    "session-377",
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
	if tkt.State != ticket.State("validated") {
		t.Fatalf("ticket state = %q, want validated", tkt.State)
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
	brief, ok, err := env.runtime.LoadBrief("TKT-377")
	if err != nil || !ok {
		t.Fatalf("expected persisted run brief, ok=%v err=%v", ok, err)
	}
	if brief.Outcome != string(agentrun.StatusDone) || brief.CommitSHA != sha {
		t.Fatalf("unexpected run brief: %#v", brief)
	}
	if brief.SessionID != "session-377" {
		t.Fatalf("brief session id = %q, want session-377", brief.SessionID)
	}
	if strings.TrimSpace(brief.CloseoutCommitSHA) == "" {
		t.Fatalf("expected closeout commit sha in brief, got %#v", brief)
	}
	if len(brief.FilesTouched) == 0 || brief.FilesTouched[0] != "feature.txt" {
		t.Fatalf("expected touched files in brief, got %#v", brief.FilesTouched)
	}
	commitBody := runGitOutput(t, env.repoRoot, "log", "-1", "--format=%B")
	for _, want := range []string{
		"docket: close out TKT-377",
		"Ticket: TKT-377",
		"Docket-Outcome: done",
		"Docket-Run-ID: session-377",
		"Docket-Implementer-Commit: " + sha,
		"Docket-Validation: passed",
		"Docket-Resume-Next:",
	} {
		if !strings.Contains(commitBody, want) {
			t.Fatalf("closeout commit missing %q in %q", want, commitBody)
		}
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
	if tkt.State != ticket.State("running") {
		t.Fatalf("ticket state = %q, want running", tkt.State)
	}
	if len(tkt.Comments) == 0 || !strings.Contains(tkt.Comments[len(tkt.Comments)-1].Body, "baseline tests failing") {
		t.Fatalf("expected stuck comment recorded, got %#v", tkt.Comments)
	}
	brief, ok, err := env.runtime.LoadBrief("TKT-377")
	if err != nil || !ok {
		t.Fatalf("expected persisted run brief, ok=%v err=%v", ok, err)
	}
	if brief.Outcome != string(agentrun.StatusStuck) {
		t.Fatalf("unexpected brief outcome: %#v", brief)
	}
	if !strings.Contains(strings.Join(brief.Decisions, "\n"), "baseline tests failing") {
		t.Fatalf("expected blocker reason in run brief, got %#v", brief)
	}
}

func TestServiceFinalizeWritesFailedValidationBriefWithoutAdvancingTicket(t *testing.T) {
	t.Parallel()

	env := buildValidationEnv(t)

	result, err := env.service.Finalize(context.Background(), agentrun.ValidationInput{
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
		t.Fatalf("Finalize() error = %v", err)
	}
	if result.Accepted {
		t.Fatalf("expected validation failure, got %#v", result)
	}
	brief, ok, err := env.runtime.LoadBrief("TKT-377")
	if err != nil || !ok {
		t.Fatalf("expected persisted validation-failure brief, ok=%v err=%v", ok, err)
	}
	if brief.Outcome != string(agentrun.StatusFailed) {
		t.Fatalf("unexpected brief outcome: %#v", brief)
	}
	if len(brief.ValidationErrors) == 0 {
		t.Fatalf("expected validation errors in brief, got %#v", brief)
	}
}

type validationEnv struct {
	repoRoot     string
	worktreePath string
	store        *local.Store
	runtime      *runruntime.Store
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
		State:       ticket.State("ready"),
		Priority:    1,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		CreatedBy:   "human:test",
		Description: "Implement the validator ticket with a bounded execution plan, explicit file targets, and concrete verification commands so the ready contract remains satisfied throughout the managed run.\n\nLikely paths:\n- feature.txt\n- README.md\n\nOut of scope:\n- changing workflow semantics\n- editing unrelated tickets\n\nVerify commands:\n- test -f feature.txt\n- test -f README.md",
		AC: []ticket.AcceptanceCriterion{
			{Description: "feature file exists", Run: "test -f feature.txt", VerificationSteps: []string{"Confirm feature.txt exists in the managed worktree."}},
			{Description: "validator flow remains documented", Run: "test -f README.md", VerificationSteps: []string{"Confirm README.md still exists after the run."}},
		},
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	flow := workflow.NewManager(store, vcs.NewGitProvider(repoRoot), claim.NewLocalClaimManager(repoRoot))
	if _, worktreePath, err := flow.StartTask(context.Background(), "TKT-377", "agent:test", cfg); err != nil {
		t.Fatalf("StartTask() error = %v", err)
	} else {
		store = local.New(repoRoot)
		runtime := runruntime.New(repoRoot)
		service := New(Dependencies{
			RepoRoot: repoRoot,
			Store:    store,
			Workflow: flow,
			Runtime:  runtime,
		})
		return validationEnv{
			repoRoot:     repoRoot,
			worktreePath: worktreePath,
			store:        store,
			runtime:      runtime,
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
