package validate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workflow"
)

type Dependencies struct {
	RepoRoot   string
	Store      *local.Store
	Workflow   *workflow.WorkflowManager
	Runtime    *runruntime.Store
	LoadConfig func(repoRoot string) (*ticket.Config, error)
	Now        func() time.Time
}

type Service struct {
	repoRoot   string
	store      *local.Store
	workflow   *workflow.WorkflowManager
	runtime    *runruntime.Store
	loadConfig func(repoRoot string) (*ticket.Config, error)
	now        func() time.Time
}

func New(deps Dependencies) *Service {
	loadConfig := deps.LoadConfig
	if loadConfig == nil {
		loadConfig = ticket.LoadConfig
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repoRoot:   deps.RepoRoot,
		store:      deps.Store,
		workflow:   deps.Workflow,
		runtime:    deps.Runtime,
		loadConfig: loadConfig,
		now:        now,
	}
}

func (s *Service) Validate(ctx context.Context, input agentrun.ValidationInput) (agentrun.ValidationResult, error) {
	reasons := make([]string, 0)
	if err := input.Result.Validate(); err != nil {
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{err.Error()}}, nil
	}
	if input.Result.Status != agentrun.StatusDone {
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{"only done results can be accepted"}}, nil
	}
	if strings.TrimSpace(input.TicketID) == "" {
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{"ticket id is required"}}, nil
	}
	if input.Result.TicketID != input.TicketID {
		reasons = append(reasons, fmt.Sprintf("result ticket %s does not match expected ticket %s", input.Result.TicketID, input.TicketID))
	}
	if input.Result.Role != agentrun.RoleImplementer {
		reasons = append(reasons, fmt.Sprintf("result role %s is not valid for implementer closeout", input.Result.Role))
	}
	ok, err := docketgit.CommitExists(input.WorktreePath, input.Result.CommitSHA)
	if err != nil {
		return agentrun.ValidationResult{}, err
	}
	if !ok {
		reasons = append(reasons, fmt.Sprintf("commit %s does not exist in worktree", input.Result.CommitSHA))
	} else if branch := strings.TrimSpace(input.Branch); branch != "" {
		onBranch, err := docketgit.IsAncestor(input.WorktreePath, input.Result.CommitSHA, branch)
		if err != nil {
			return agentrun.ValidationResult{}, err
		}
		if !onBranch {
			reasons = append(reasons, fmt.Sprintf("commit %s is not reachable from branch %s", input.Result.CommitSHA, branch))
		}
	}
	clean, err := docketgit.IsClean(input.WorktreePath)
	if err != nil {
		return agentrun.ValidationResult{}, err
	}
	if !clean {
		reasons = append(reasons, "worktree is not clean")
	}
	if acErr := s.runAcceptanceCommands(ctx, input); acErr != nil {
		reasons = append(reasons, acErr.Error())
	}
	for _, validationErr := range s.validationErrors(input.TicketID) {
		reasons = append(reasons, validationErr.Message)
	}
	return agentrun.ValidationResult{
		Accepted: len(reasons) == 0,
		Reasons:  reasons,
	}, nil
}

func (s *Service) Finalize(ctx context.Context, input agentrun.ValidationInput) (agentrun.ValidationResult, error) {
	if input.Result.Status != agentrun.StatusDone {
		_ = s.writeRecoverableStatus(input, input.Result.Status, strings.TrimSpace(input.Result.Reason))
		if err := s.recordOutcomeComment(ctx, input); err != nil {
			return agentrun.ValidationResult{}, err
		}
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{input.Result.Reason}}, nil
	}

	result, err := s.Validate(ctx, input)
	if err != nil {
		return result, err
	}
	if !result.Accepted {
		_ = s.writeRecoverableStatus(input, agentrun.StatusFailed, strings.Join(result.Reasons, "; "))
		_ = s.writeRunBrief(runruntime.RunBrief{
			TicketID:         input.TicketID,
			Outcome:          string(agentrun.StatusFailed),
			Summary:          "Managed run failed validation before closeout.",
			SessionID:        strings.TrimSpace(input.SessionID),
			CommitSHA:        strings.TrimSpace(input.Result.CommitSHA),
			Tests:            strings.TrimSpace(input.Result.Tests),
			ValidationErrors: append([]string(nil), result.Reasons...),
			ResumeNext:       "Inspect the validation failures, repair the worktree, and rerun the ticket.",
			UpdatedAt:        s.now().UTC().Format(time.RFC3339),
		})
		return result, err
	}
	brief, err := s.prepareSuccessfulHandoff(ctx, input)
	if err != nil {
		return agentrun.ValidationResult{}, err
	}
	cfg, err := s.loadConfig(s.repoRoot)
	if err != nil {
		return agentrun.ValidationResult{}, err
	}
	mergeMessage := buildManagedCloseoutCommitMessage(brief)
	if _, err := s.workflow.FinishTaskWithSummary(ctx, input.TicketID, cfg, mergeMessage); err != nil {
		return agentrun.ValidationResult{}, err
	}
	if closeoutSHA, shaErr := docketgit.HeadSHA(s.repoRoot); shaErr == nil {
		brief.CloseoutCommitSHA = strings.TrimSpace(closeoutSHA)
		_ = s.writeRunBrief(brief)
	}
	if len(s.validationErrors(input.TicketID)) > 0 {
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{"ticket invalid after validated transition"}}, nil
	}
	return result, nil
}

func (s *Service) runAcceptanceCommands(ctx context.Context, input agentrun.ValidationInput) error {
	tkt, err := s.store.GetTicket(ctx, input.TicketID)
	if err != nil {
		return err
	}
	if tkt == nil {
		return fmt.Errorf("ticket %s not found", input.TicketID)
	}
	for _, ac := range tkt.AC {
		command := strings.TrimSpace(ac.Run)
		if command == "" {
			continue
		}
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = input.WorktreePath
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("acceptance check failed for %q: %s", ac.Description, strings.TrimSpace(out.String()))
		}
	}
	return nil
}

func (s *Service) validationErrors(ticketID string) []store.ValidationError {
	if s.store == nil {
		return nil
	}
	errs, _, err := s.store.ValidateFile(ticketID)
	if err != nil {
		return []store.ValidationError{{Field: "ticket", Message: err.Error()}}
	}
	return errs
}

func (s *Service) prepareSuccessfulHandoff(ctx context.Context, input agentrun.ValidationInput) (runruntime.RunBrief, error) {
	tkt, err := s.store.GetTicket(ctx, input.TicketID)
	if err != nil {
		return runruntime.RunBrief{}, err
	}
	if tkt == nil {
		return runruntime.RunBrief{}, fmt.Errorf("ticket %s not found", input.TicketID)
	}
	for _, existing := range tkt.LinkedCommits {
		if existing == input.Result.CommitSHA {
			goto comment
		}
	}
	tkt.LinkedCommits = append(tkt.LinkedCommits, input.Result.CommitSHA)
comment:
	tkt.Comments = append(tkt.Comments, ticket.Comment{
		At:     s.now().UTC().Truncate(time.Second),
		Author: "agent:docket-runner",
		Body:   fmt.Sprintf("Managed run completed with commit %s and tests=%s.", input.Result.CommitSHA, strings.TrimSpace(input.Result.Tests)),
	})
	tkt.Handoff = buildHandoff(input)
	tkt.UpdatedAt = s.now().UTC().Truncate(time.Second)
	if err := s.store.UpdateTicket(ctx, tkt); err != nil {
		return runruntime.RunBrief{}, err
	}
	filesTouched, filesErr := touchedFilesForCommit(input.WorktreePath, input.Result.CommitSHA)
	if filesErr != nil {
		filesTouched = nil
	}
	brief := runruntime.RunBrief{
		TicketID:     input.TicketID,
		Outcome:      string(agentrun.StatusDone),
		Summary:      "Managed run validated successfully and advanced the ticket.",
		SessionID:    strings.TrimSpace(input.SessionID),
		CommitSHA:    input.Result.CommitSHA,
		Tests:        strings.TrimSpace(input.Result.Tests),
		FilesTouched: filesTouched,
		Decisions: []string{
			fmt.Sprintf("Accepted implementer result at commit %s.", input.Result.CommitSHA),
			"Moved ticket to the configured validated/completed workflow state.",
		},
		ResumeNext: "Archive the ticket or reopen it only if follow-up work is still required.",
		UpdatedAt:  s.now().UTC().Format(time.RFC3339),
	}
	if err := s.writeRunBrief(brief); err != nil {
		return runruntime.RunBrief{}, err
	}
	return brief, nil
}

func (s *Service) recordOutcomeComment(ctx context.Context, input agentrun.ValidationInput) error {
	tkt, err := s.store.GetTicket(ctx, input.TicketID)
	if err != nil {
		return err
	}
	if tkt == nil {
		return fmt.Errorf("ticket %s not found", input.TicketID)
	}
	tkt.Comments = append(tkt.Comments, ticket.Comment{
		At:     s.now().UTC().Truncate(time.Second),
		Author: "agent:docket-runner",
		Body:   fmt.Sprintf("Managed run reported %s: %s", input.Result.Status, strings.TrimSpace(input.Result.Reason)),
	})
	tkt.UpdatedAt = s.now().UTC().Truncate(time.Second)
	if err := s.store.UpdateTicket(ctx, tkt); err != nil {
		return err
	}
	return s.writeRunBrief(runruntime.RunBrief{
		TicketID:   input.TicketID,
		Outcome:    string(input.Result.Status),
		Summary:    fmt.Sprintf("Managed run reported %s and left the ticket in its active state.", input.Result.Status),
		SessionID:  strings.TrimSpace(input.SessionID),
		Tests:      strings.TrimSpace(input.Result.Tests),
		Decisions:  []string{strings.TrimSpace(input.Result.Reason)},
		ResumeNext: "Inspect the run status, address the blocker, and resume the managed run when ready.",
		UpdatedAt:  s.now().UTC().Format(time.RFC3339),
	})
}

func buildHandoff(input agentrun.ValidationInput) string {
	return fmt.Sprintf(
		"*Last updated: %s by agent:docket-runner*\n\n**Current state:** Validated and merged back to the main checkout.\n\n**Decisions made:** Managed run finished successfully at commit %s.\n\n**Files touched:** See commit %s.\n\n**Remaining work:** Archive or reopen only if follow-up work is required.\n\n**AC status:** %s.",
		time.Now().UTC().Format(time.RFC3339),
		input.Result.CommitSHA,
		input.Result.CommitSHA,
		strings.TrimSpace(input.Result.Tests),
	)
}

func buildManagedCloseoutCommitMessage(brief runruntime.RunBrief) string {
	subjectTicket := strings.TrimSpace(brief.TicketID)
	if subjectTicket == "" {
		subjectTicket = "ticket"
	}
	lines := []string{
		fmt.Sprintf("docket: close out %s", subjectTicket),
		"",
		fmt.Sprintf("Ticket: %s", brief.TicketID),
		fmt.Sprintf("Docket-Outcome: %s", strings.TrimSpace(brief.Outcome)),
	}
	if strings.TrimSpace(brief.SessionID) != "" {
		lines = append(lines, fmt.Sprintf("Docket-Run-ID: %s", strings.TrimSpace(brief.SessionID)))
	}
	if strings.TrimSpace(brief.CommitSHA) != "" {
		lines = append(lines, fmt.Sprintf("Docket-Implementer-Commit: %s", strings.TrimSpace(brief.CommitSHA)))
	}
	if strings.TrimSpace(brief.Summary) != "" {
		lines = append(lines, fmt.Sprintf("Docket-Summary: %s", singleLineCommitField(brief.Summary)))
	}
	if strings.TrimSpace(brief.Tests) != "" {
		lines = append(lines, fmt.Sprintf("Docket-Validation: %s", singleLineCommitField(brief.Tests)))
	}
	if len(brief.FilesTouched) > 0 {
		lines = append(lines, fmt.Sprintf("Docket-Files: %s", singleLineCommitField(strings.Join(brief.FilesTouched, ", "))))
	}
	if len(brief.Decisions) > 0 {
		lines = append(lines, fmt.Sprintf("Docket-Decisions: %s", singleLineCommitField(strings.Join(brief.Decisions, " | "))))
	}
	if strings.TrimSpace(brief.ResumeNext) != "" {
		lines = append(lines, fmt.Sprintf("Docket-Resume-Next: %s", singleLineCommitField(brief.ResumeNext)))
	}
	lines = append(lines, durableRunSummaryBlock(brief)...)
	return strings.Join(lines, "\n")
}

func durableRunSummaryBlock(brief runruntime.RunBrief) []string {
	lines := []string{"", "Docket-Run-Summary:"}
	if ticketID := strings.TrimSpace(brief.TicketID); ticketID != "" {
		lines = append(lines, fmt.Sprintf("  ticket: %s", ticketID))
	}
	if outcome := strings.TrimSpace(brief.Outcome); outcome != "" {
		lines = append(lines, fmt.Sprintf("  outcome: %s", outcome))
	}
	if validation := strings.TrimSpace(brief.Tests); validation != "" {
		lines = append(lines, fmt.Sprintf("  validation: %s", singleLineCommitField(validation)))
	}
	if next := strings.TrimSpace(brief.ResumeNext); next != "" {
		lines = append(lines, fmt.Sprintf("  next: %s", singleLineCommitField(next)))
	}
	return lines
}

func singleLineCommitField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func (s *Service) writeRunBrief(brief runruntime.RunBrief) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.WriteBrief(brief)
}

func (s *Service) writeRecoverableStatus(input agentrun.ValidationInput, outcome agentrun.Status, detail string) error {
	if s.runtime == nil {
		return nil
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	status, ok, err := s.runtime.LoadStatus(input.TicketID)
	if err != nil {
		return err
	}
	if !ok {
		status = runruntime.StatusSnapshot{
			TicketID:  input.TicketID,
			SessionID: strings.TrimSpace(input.SessionID),
		}
	}
	if strings.TrimSpace(status.SessionID) == "" {
		status.SessionID = strings.TrimSpace(input.SessionID)
	}
	status.Active = false
	status.Hung = false
	status.PID = 0
	status.LastEventAt = now
	status.LastVisibleAt = now
	status.LastResultStatus = string(outcome)
	if strings.TrimSpace(detail) != "" {
		status.LastVisibleText = detail
	}
	return s.runtime.WriteStatus(status)
}

func touchedFilesForCommit(repoRoot, commitSHA string) ([]string, error) {
	if strings.TrimSpace(commitSHA) == "" {
		return nil, nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "show", "--pretty=", "--name-only", commitSHA)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	files := make([]string, 0)
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}
