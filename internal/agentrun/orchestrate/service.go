package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/security"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workflow"
)

type Dependencies struct {
	RepoRoot   string
	Actor      string
	Store      *local.Store
	Workflow   *workflow.WorkflowManager
	Namespace  *security.RepoNamespaceStore
	Adapter    agentrun.Adapter
	Reviewer   agentrun.Adapter
	Monitor    agentrun.Monitor
	Validator  agentrun.Validator
	Selector   agentrun.Selector
	Runtime    *runruntime.Store
	Timeout    time.Duration
	LoadConfig func(repoRoot string) (*ticket.Config, error)
}

type Service struct {
	repoRoot   string
	actor      string
	store      *local.Store
	workflow   *workflow.WorkflowManager
	namespace  *security.RepoNamespaceStore
	adapter    agentrun.Adapter
	reviewer   agentrun.Adapter
	monitor    agentrun.Monitor
	validator  agentrun.Validator
	selector   agentrun.Selector
	runtime    *runruntime.Store
	timeout    time.Duration
	loadConfig func(repoRoot string) (*ticket.Config, error)
}

type StartedRun struct {
	Handle       agentrun.ProcessHandle
	Record       agentrun.RunRecord
	WorktreePath string
	Branch       string
}

func New(deps Dependencies) *Service {
	loadConfig := deps.LoadConfig
	if loadConfig == nil {
		loadConfig = ticket.LoadConfig
	}
	return &Service{
		repoRoot:   deps.RepoRoot,
		actor:      deps.Actor,
		store:      deps.Store,
		workflow:   deps.Workflow,
		namespace:  deps.Namespace,
		adapter:    deps.Adapter,
		reviewer:   deps.Reviewer,
		monitor:    deps.Monitor,
		validator:  deps.Validator,
		selector:   deps.Selector,
		runtime:    deps.Runtime,
		timeout:    deps.Timeout,
		loadConfig: loadConfig,
	}
}

func (s *Service) StartImplementer(ctx context.Context, ticketID string) (StartedRun, error) {
	if s.workflow == nil {
		return StartedRun{}, fmt.Errorf("workflow is required")
	}
	if s.adapter == nil {
		return StartedRun{}, fmt.Errorf("adapter is required")
	}
	if s.namespace == nil {
		return StartedRun{}, fmt.Errorf("namespace store is required")
	}
	cfg, err := s.loadConfig(s.repoRoot)
	if err != nil {
		return StartedRun{}, err
	}
	_, worktreePath, err := s.workflow.StartTask(ctx, ticketID, s.actor, cfg)
	if err != nil {
		return StartedRun{}, err
	}
	branch := "docket/" + ticketID
	if err := s.namespace.RecordRunStart(s.repoRoot, ticketID, s.actor, worktreePath, branch, ""); err != nil {
		return StartedRun{}, err
	}
	tkt, err := s.store.GetTicket(ctx, ticketID)
	if err != nil {
		return StartedRun{}, err
	}
	spec := agentrun.RunSpec{
		TicketID:     ticketID,
		Role:         agentrun.RoleImplementer,
		RepoRoot:     s.repoRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		Prompt:       agentrun.DefaultImplementerPrompt(tkt),
	}
	handle, record, err := s.adapter.Start(ctx, spec)
	if err != nil {
		return StartedRun{}, err
	}
	if err := agentrun.WriteRunRecord(s.repoRoot, record); err != nil {
		return StartedRun{}, err
	}
	if s.runtime != nil {
		if err := s.runtime.Init(record, spec.Prompt, s.monitorTimeout()); err != nil {
			return StartedRun{}, err
		}
	}
	return StartedRun{
		Handle:       handle,
		Record:       record,
		WorktreePath: worktreePath,
		Branch:       branch,
	}, nil
}

func (s *Service) RunTicket(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
	if s.monitor == nil {
		return agentrun.TicketRunSummary{}, fmt.Errorf("monitor is required")
	}
	if s.validator == nil {
		return agentrun.TicketRunSummary{}, fmt.Errorf("validator is required")
	}
	started, err := s.StartImplementer(ctx, ticketID)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	obs, err := s.monitor.Observe(ctx, agentrun.ObservationInput{
		Handle:  started.Handle,
		Record:  started.Record,
		Timeout: s.monitorTimeout(),
	})
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	validationInput := agentrun.ValidationInput{
		TicketID:     ticketID,
		RepoRoot:     s.repoRoot,
		WorktreePath: started.WorktreePath,
		Branch:       started.Branch,
		Result:       obs.Result,
	}
	if obs.Result.Status != agentrun.StatusDone {
		validation, err := s.validator.Finalize(ctx, validationInput)
		if err != nil {
			return agentrun.TicketRunSummary{}, err
		}
		if !obs.TimedOut {
			_ = s.cleanupRuntime(ticketID)
		}
		if obs.TimedOut {
			return failedOrRawSummary(ticketID, obs.Result.Status, "run hung; inspect with `docket run-status` and continue with `docket run-resume`", validation), nil
		}
		return failedOrRawSummary(ticketID, obs.Result.Status, obs.Result.Reason, validation), nil
	}
	validation, err := s.validator.Validate(ctx, validationInput)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	if !validation.Accepted {
		_ = s.cleanupRuntime(ticketID)
		return failedOrRawSummary(ticketID, agentrun.StatusFailed, strings.Join(validation.Reasons, "; "), validation), nil
	}
	if s.reviewer != nil {
		review, currentResult, err := s.runReviewerLoop(ctx, ticketID, started.WorktreePath, started.Branch, obs.Result)
		if err != nil {
			return agentrun.TicketRunSummary{}, err
		}
		if review.Status != agentrun.ReviewApproved {
			_ = s.cleanupRuntime(ticketID)
			return agentrun.TicketRunSummary{
				TicketID: ticketID,
				Status:   agentrun.StatusFailed,
				Reason:   review.RequiredChanges,
			}, nil
		}
		validationInput.Result = currentResult
	}
	validation, err = s.validator.Finalize(ctx, validationInput)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	summary := agentrun.TicketRunSummary{
		TicketID: ticketID,
		Status:   agentrun.StatusDone,
	}
	if !validation.Accepted {
		summary.Status = agentrun.StatusFailed
		summary.Reason = strings.Join(validation.Reasons, "; ")
	} else {
		summary.Reason = "validated and advanced"
	}
	_ = s.cleanupRuntime(ticketID)
	return summary, nil
}

func (s *Service) RunNext(ctx context.Context) (agentrun.CycleSummary, error) {
	if s.selector == nil {
		return agentrun.CycleSummary{}, fmt.Errorf("selector is required")
	}
	var summary agentrun.CycleSummary
	for {
		selection, err := s.selector.Next(ctx)
		if err != nil {
			return summary, err
		}
		if !selection.Found {
			summary.StopReason = selection.Reason
			if summary.StopReason == "" {
				summary.StopReason = "no runnable tickets remain"
			}
			return summary, nil
		}
		runSummary, err := s.RunTicket(ctx, selection.TicketID)
		if err != nil {
			return summary, err
		}
		summary.Runs = append(summary.Runs, runSummary)
		if runSummary.Status != agentrun.StatusDone {
			if runSummary.Reason != "" {
				summary.StopReason = runSummary.Reason
			} else {
				summary.StopReason = fmt.Sprintf("stopped after %s returned %s", runSummary.TicketID, runSummary.Status)
			}
			return summary, nil
		}
	}
}

func (s *Service) ResumeTicket(ctx context.Context, ticketID string) (agentrun.TicketRunSummary, error) {
	if s.runtime == nil {
		return agentrun.TicketRunSummary{}, fmt.Errorf("runtime store is required")
	}
	status, ok, err := s.runtime.LoadStatus(ticketID)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	if !ok || !status.Hung {
		return agentrun.TicketRunSummary{}, fmt.Errorf("ticket %s does not have a hung active run", ticketID)
	}
	prompt, err := s.runtime.LoadPrompt(ticketID)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	transcript, err := s.runtime.LoadTranscript(ticketID)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	tkt, err := s.store.GetTicket(ctx, ticketID)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	worktreePath := ""
	branch := "docket/" + ticketID
	if manifest, ok, err := s.namespace.GetRunManifest(s.repoRoot, ticketID); err == nil && ok {
		worktreePath = manifest.WorktreePath
		if manifest.Branch != "" {
			branch = manifest.Branch
		}
	}
	if strings.TrimSpace(worktreePath) == "" {
		return agentrun.TicketRunSummary{}, fmt.Errorf("no active worktree recorded for %s", ticketID)
	}
	resumePrompt := buildResumePrompt(prompt, tkt, transcript, status)
	started, err := s.startFollowup(ctx, ticketID, worktreePath, branch, agentrun.RoleImplementer, resumePrompt, s.adapter)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	obs, err := s.monitor.Observe(ctx, agentrun.ObservationInput{
		Handle:  started.Handle,
		Record:  started.Record,
		Timeout: s.monitorTimeout(),
	})
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	validation, err := s.validator.Finalize(ctx, agentrun.ValidationInput{
		TicketID:     ticketID,
		RepoRoot:     s.repoRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		Result:       obs.Result,
	})
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	if !obs.TimedOut {
		_ = s.cleanupRuntime(ticketID)
	}
	if obs.TimedOut {
		return failedOrRawSummary(ticketID, obs.Result.Status, "run hung again; inspect with `docket run-status`", validation), nil
	}
	return failedOrRawSummary(ticketID, obs.Result.Status, obs.Result.Reason, validation), nil
}

func (s *Service) startFollowup(ctx context.Context, ticketID, worktreePath, branch string, role agentrun.Role, prompt string, adapter agentrun.Adapter) (StartedRun, error) {
	if adapter == nil {
		return StartedRun{}, fmt.Errorf("adapter is required")
	}
	if err := s.namespace.RecordRunStart(s.repoRoot, ticketID, s.actor, worktreePath, branch, ""); err != nil {
		return StartedRun{}, err
	}
	spec := agentrun.RunSpec{
		TicketID:     ticketID,
		Role:         role,
		RepoRoot:     s.repoRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		Prompt:       prompt,
	}
	handle, record, err := adapter.Start(ctx, spec)
	if err != nil {
		return StartedRun{}, err
	}
	if err := agentrun.WriteRunRecord(s.repoRoot, record); err != nil {
		return StartedRun{}, err
	}
	if s.runtime != nil {
		if err := s.runtime.Init(record, spec.Prompt, s.monitorTimeout()); err != nil {
			return StartedRun{}, err
		}
	}
	return StartedRun{
		Handle:       handle,
		Record:       record,
		WorktreePath: worktreePath,
		Branch:       branch,
	}, nil
}

func (s *Service) runReviewerLoop(ctx context.Context, ticketID, worktreePath, branch string, currentResult agentrun.Result) (agentrun.ReviewResult, agentrun.Result, error) {
	review, err := s.runReview(ctx, ticketID, worktreePath, branch)
	if err != nil {
		return agentrun.ReviewResult{}, currentResult, err
	}
	if review.Status == agentrun.ReviewApproved {
		return review, currentResult, nil
	}

	fixStarted, err := s.startFollowup(ctx, ticketID, worktreePath, branch, agentrun.RoleImplementer, agentrun.DefaultFixPrompt(ticketID, review.RequiredChanges), s.adapter)
	if err != nil {
		return agentrun.ReviewResult{}, currentResult, err
	}
	fixObs, err := s.monitor.Observe(ctx, agentrun.ObservationInput{
		Handle:  fixStarted.Handle,
		Record:  fixStarted.Record,
		Timeout: s.monitorTimeout(),
	})
	if err != nil {
		return agentrun.ReviewResult{}, currentResult, err
	}
	if fixObs.Result.Status != agentrun.StatusDone {
		return agentrun.ReviewResult{Status: agentrun.ReviewChangesRequired, TicketID: ticketID, Role: agentrun.RoleReviewer, RequiredChanges: fixObs.Result.Reason}, currentResult, nil
	}
	validation, err := s.validator.Validate(ctx, agentrun.ValidationInput{
		TicketID:     ticketID,
		RepoRoot:     s.repoRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		Result:       fixObs.Result,
	})
	if err != nil {
		return agentrun.ReviewResult{}, currentResult, err
	}
	if !validation.Accepted {
		return agentrun.ReviewResult{Status: agentrun.ReviewChangesRequired, TicketID: ticketID, Role: agentrun.RoleReviewer, RequiredChanges: strings.Join(validation.Reasons, "; ")}, currentResult, nil
	}
	secondReview, err := s.runReview(ctx, ticketID, worktreePath, branch)
	if err != nil {
		return agentrun.ReviewResult{}, currentResult, err
	}
	return secondReview, fixObs.Result, nil
}

func (s *Service) runReview(ctx context.Context, ticketID, worktreePath, branch string) (agentrun.ReviewResult, error) {
	started, err := s.startFollowup(ctx, ticketID, worktreePath, branch, agentrun.RoleReviewer, agentrun.DefaultReviewerPrompt(ticketID), s.reviewer)
	if err != nil {
		return agentrun.ReviewResult{}, err
	}
	obs, err := s.monitor.Observe(ctx, agentrun.ObservationInput{
		Handle:  started.Handle,
		Record:  started.Record,
		Timeout: s.monitorTimeout(),
	})
	if err != nil {
		return agentrun.ReviewResult{}, err
	}
	if obs.Review == nil {
		return agentrun.ReviewResult{
			Status:          agentrun.ReviewChangesRequired,
			TicketID:        ticketID,
			Role:            agentrun.RoleReviewer,
			RequiredChanges: "reviewer did not emit a REVIEW line",
		}, nil
	}
	return *obs.Review, nil
}

func failedOrRawSummary(ticketID string, status agentrun.Status, fallbackReason string, validation agentrun.ValidationResult) agentrun.TicketRunSummary {
	reason := fallbackReason
	if len(validation.Reasons) > 0 {
		reason = strings.Join(validation.Reasons, "; ")
	}
	return agentrun.TicketRunSummary{
		TicketID: ticketID,
		Status:   status,
		Reason:   reason,
	}
}

func (s *Service) monitorTimeout() time.Duration {
	if s.timeout > 0 {
		return s.timeout
	}
	return 10 * time.Minute
}

func (s *Service) cleanupRuntime(ticketID string) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.Cleanup(ticketID)
}

func buildResumePrompt(originalPrompt string, tkt *ticket.Ticket, transcript []runruntime.TranscriptEntry, status runruntime.StatusSnapshot) string {
	start := 0
	if len(transcript) > 8 {
		start = len(transcript) - 8
	}
	lines := make([]string, 0, len(transcript[start:]))
	for _, entry := range transcript[start:] {
		if strings.TrimSpace(entry.Text) == "" {
			continue
		}
		lines = append(lines, "- "+entry.Text)
	}
	title := ""
	ticketID := ""
	if tkt != nil {
		ticketID = tkt.ID
		title = strings.TrimSpace(tkt.Title)
	} else {
		ticketID = status.TicketID
	}
	step := status.CurrentPhase
	if status.CurrentStepTitle != "" {
		step = fmt.Sprintf("%d/%d %s", status.CurrentStep, status.PlannedSteps, status.CurrentStepTitle)
	}
	return strings.TrimSpace(originalPrompt) + "\n\nPrevious run hung before completion.\nContinue from the current worktree state instead of restarting.\nTicket: " + ticketID + "\nTitle: " + title + "\nLast known progress: " + step + "\nRecent visible transcript:\n" + strings.Join(lines, "\n")
}
