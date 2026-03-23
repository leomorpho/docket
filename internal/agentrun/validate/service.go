package validate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
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
	LoadConfig func(repoRoot string) (*ticket.Config, error)
	Now        func() time.Time
}

type Service struct {
	repoRoot   string
	store      *local.Store
	workflow   *workflow.WorkflowManager
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
		if err := s.recordOutcomeComment(ctx, input); err != nil {
			return agentrun.ValidationResult{}, err
		}
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{input.Result.Reason}}, nil
	}

	result, err := s.Validate(ctx, input)
	if err != nil || !result.Accepted {
		return result, err
	}
	if err := s.prepareSuccessfulHandoff(ctx, input); err != nil {
		return agentrun.ValidationResult{}, err
	}
	cfg, err := s.loadConfig(s.repoRoot)
	if err != nil {
		return agentrun.ValidationResult{}, err
	}
	if _, err := s.workflow.FinishTask(ctx, input.TicketID, cfg); err != nil {
		return agentrun.ValidationResult{}, err
	}
	if len(s.validationErrors(input.TicketID)) > 0 {
		return agentrun.ValidationResult{Accepted: false, Reasons: []string{"ticket invalid after review transition"}}, nil
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

func (s *Service) prepareSuccessfulHandoff(ctx context.Context, input agentrun.ValidationInput) error {
	tkt, err := s.store.GetTicket(ctx, input.TicketID)
	if err != nil {
		return err
	}
	if tkt == nil {
		return fmt.Errorf("ticket %s not found", input.TicketID)
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
	return s.store.UpdateTicket(ctx, tkt)
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
	return s.store.UpdateTicket(ctx, tkt)
}

func buildHandoff(input agentrun.ValidationInput) string {
	return fmt.Sprintf(
		"*Last updated: %s by agent:docket-runner*\n\n**Current state:** in-review.\n\n**Decisions made:** Managed run finished successfully at commit %s.\n\n**Files touched:** See commit %s.\n\n**Remaining work:** Human review.\n\n**AC status:** %s.",
		time.Now().UTC().Format(time.RFC3339),
		input.Result.CommitSHA,
		input.Result.CommitSHA,
		strings.TrimSpace(input.Result.Tests),
	)
}
