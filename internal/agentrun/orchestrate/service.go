package orchestrate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	"github.com/leomorpho/docket/internal/workflow"
)

type Dependencies struct {
	RepoRoot       string
	Actor          string
	Store          *local.Store
	Workflow       *workflow.WorkflowManager
	Namespace      *runstate.Store
	Adapter        agentrun.Adapter
	Reviewer       agentrun.Adapter
	Monitor        agentrun.Monitor
	Validator      agentrun.Validator
	Selector       agentrun.Selector
	Runtime        *runruntime.Store
	Timeout        time.Duration
	MaxAutoResumes int
	LoadConfig     func(repoRoot string) (*ticket.Config, error)
}

type Service struct {
	repoRoot       string
	actor          string
	store          *local.Store
	workflow       *workflow.WorkflowManager
	namespace      *runstate.Store
	adapter        agentrun.Adapter
	reviewer       agentrun.Adapter
	monitor        agentrun.Monitor
	validator      agentrun.Validator
	selector       agentrun.Selector
	runtime        *runruntime.Store
	timeout        time.Duration
	maxAutoResumes int
	loadConfig     func(repoRoot string) (*ticket.Config, error)
}

type StartedRun struct {
	Handle       agentrun.ProcessHandle
	Record       agentrun.RunRecord
	WorktreePath string
	Branch       string
}

type reviewerContractError struct {
	reason string
}

const (
	defaultMonitorTimeout = 2 * time.Minute
	defaultMaxAutoResumes = 2
)

func (e reviewerContractError) Error() string {
	return e.reason
}

func New(deps Dependencies) *Service {
	loadConfig := deps.LoadConfig
	if loadConfig == nil {
		loadConfig = ticket.LoadConfig
	}
	return &Service{
		repoRoot:       deps.RepoRoot,
		actor:          deps.Actor,
		store:          deps.Store,
		workflow:       deps.Workflow,
		namespace:      deps.Namespace,
		adapter:        deps.Adapter,
		reviewer:       deps.Reviewer,
		monitor:        deps.Monitor,
		validator:      deps.Validator,
		selector:       deps.Selector,
		runtime:        deps.Runtime,
		timeout:        deps.Timeout,
		maxAutoResumes: deps.MaxAutoResumes,
		loadConfig:     loadConfig,
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
		if err := s.runtime.Continue(record, spec.Prompt, s.monitorTimeout()); err != nil {
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
	if err == nil && obs.TimedOut && s.runtime != nil {
		obs, err = s.resumeTimedOutImplementer(ctx, ticketID, started.WorktreePath, started.Branch, 1)
	}
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	validationInput := agentrun.ValidationInput{
		TicketID:     ticketID,
		RepoRoot:     s.repoRoot,
		WorktreePath: started.WorktreePath,
		Branch:       started.Branch,
		SessionID:    started.Record.SessionID,
		Result:       obs.Result,
	}
	if obs.Result.Status != agentrun.StatusDone {
		validation, err := s.validator.Finalize(ctx, validationInput)
		if err != nil {
			return agentrun.TicketRunSummary{}, err
		}
		if obs.TimedOut {
			reason := strings.TrimSpace(obs.Result.Reason)
			if reason == "" {
				reason = "run hung; inspect with `docket run-status` and continue with `docket run-resume`"
			}
			return failedOrRawSummary(ticketID, obs.Result.Status, reason, validation), nil
		}
		if isRecoverableManagedRunResult(obs.Result.Status) {
			_ = s.markRuntimeRecoverable(ticketID, started.Record.SessionID, obs.Result.Status, obs.Result.Reason)
		}
		if !isRecoverableManagedRunResult(obs.Result.Status) {
			_ = s.cleanupRuntime(ticketID)
		}
		return failedOrRawSummary(ticketID, obs.Result.Status, obs.Result.Reason, validation), nil
	}
	validation, err := s.validator.Validate(ctx, validationInput)
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	if !validation.Accepted {
		_ = s.markRuntimeRecoverable(ticketID, started.Record.SessionID, agentrun.StatusFailed, strings.Join(validation.Reasons, "; "))
		_ = s.persistValidationFailureBrief(validationInput, validation)
		return failedOrRawSummary(ticketID, agentrun.StatusFailed, strings.Join(validation.Reasons, "; "), validation), nil
	}
	if s.reviewer != nil {
		review, currentResult, err := s.runReviewerLoop(ctx, ticketID, started.WorktreePath, started.Branch, obs.Result)
		if err != nil {
			var reviewErr reviewerContractError
			if errors.As(err, &reviewErr) {
				_ = s.cleanupRuntime(ticketID)
				return agentrun.TicketRunSummary{
					TicketID: ticketID,
					Status:   agentrun.StatusFailed,
					Reason:   reviewErr.reason,
				}, nil
			}
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
	if s.runtime != nil {
		if err := s.runtime.BeginCycle(time.Now()); err != nil {
			return agentrun.CycleSummary{}, err
		}
		defer func() { _ = s.runtime.EndCycle() }()
	}
	var summary agentrun.CycleSummary
	for {
		if s.runtime != nil {
			stopRequested, err := s.runtime.StopAfterCurrentRequested()
			if err != nil {
				return summary, err
			}
			if stopRequested {
				if len(summary.Runs) == 0 {
					summary.StopReason = "operator requested stop before starting the next ticket"
				} else {
					summary.StopReason = "operator requested stop after current ticket"
				}
				return summary, nil
			}
		}
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
		if s.runtime != nil {
			if err := s.runtime.UpdateCycleCurrent(selection.TicketID, time.Now()); err != nil {
				return summary, err
			}
		}
		runStartedAt := time.Now()
		runSummary, err := s.RunTicket(ctx, selection.TicketID)
		if err != nil {
			return summary, err
		}
		summary.Runs = append(summary.Runs, runSummary)
		if s.runtime != nil && runSummary.Status == agentrun.StatusDone {
			if err := s.runtime.AppendCycleCompleted(selection.TicketID, string(runSummary.Status), formatRunLength(time.Since(runStartedAt)), time.Now()); err != nil {
				return summary, err
			}
		}
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

func formatRunLength(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	switch {
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
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
	if !ok {
		status, ok, err = s.runtime.LoadRecoverableStatus(ticketID)
		if err != nil {
			return agentrun.TicketRunSummary{}, err
		}
	}
	if !ok || !isRecoverableManagedRunStatus(status) {
		return agentrun.TicketRunSummary{}, fmt.Errorf("ticket %s does not have a recoverable managed run", ticketID)
	}
	prompt, err := s.runtime.LoadPrompt(ticketID)
	if err != nil {
		if !os.IsNotExist(err) {
			return agentrun.TicketRunSummary{}, err
		}
		prompt = ""
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
	started, err := s.startImplementerContinuation(ctx, ticketID, worktreePath, branch, status.SessionID, resumePrompt)
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
		SessionID:    status.SessionID,
		Result:       obs.Result,
	})
	if err != nil {
		return agentrun.TicketRunSummary{}, err
	}
	if !obs.TimedOut {
		if isRecoverableManagedRunResult(obs.Result.Status) {
			_ = s.markRuntimeRecoverable(ticketID, status.SessionID, obs.Result.Status, obs.Result.Reason)
		}
		if !isRecoverableManagedRunResult(obs.Result.Status) {
			_ = s.cleanupRuntime(ticketID)
		}
	}
	if obs.TimedOut {
		return failedOrRawSummary(ticketID, obs.Result.Status, "run hung again; inspect with `docket run-status`", validation), nil
	}
	return failedOrRawSummary(ticketID, obs.Result.Status, obs.Result.Reason, validation), nil
}

func isRecoverableManagedRunResult(status agentrun.Status) bool {
	return status == agentrun.StatusStuck || status == agentrun.StatusFailed
}

func isRecoverableManagedRunStatus(status runruntime.StatusSnapshot) bool {
	if strings.TrimSpace(status.SessionID) == "" {
		return false
	}
	if status.Hung {
		return true
	}
	switch strings.TrimSpace(status.LastResultStatus) {
	case string(agentrun.StatusStuck), string(agentrun.StatusFailed):
		return true
	default:
		return false
	}
}

func (s *Service) markRuntimeRecoverable(ticketID, sessionID string, result agentrun.Status, detail string) error {
	if s.runtime == nil {
		return nil
	}
	status, ok, err := s.runtime.LoadStatus(ticketID)
	if err != nil {
		return err
	}
	if !ok {
		status = runruntime.StatusSnapshot{TicketID: ticketID}
	}
	if strings.TrimSpace(status.SessionID) == "" {
		status.SessionID = strings.TrimSpace(sessionID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status.Active = false
	status.Hung = false
	status.PID = 0
	status.LastEventAt = now
	status.LastVisibleAt = now
	status.LastResultStatus = string(result)
	if strings.TrimSpace(detail) != "" {
		status.LastVisibleText = strings.TrimSpace(detail)
	}
	return s.runtime.WriteStatus(status)
}

func (s *Service) PingTicket(ctx context.Context, ticketID string) (agentrun.PingSummary, error) {
	if s.runtime == nil {
		return agentrun.PingSummary{}, fmt.Errorf("runtime store is required")
	}
	status, ok, err := s.runtime.LoadStatus(ticketID)
	if err != nil {
		return agentrun.PingSummary{}, err
	}
	if !ok {
		return agentrun.PingSummary{}, fmt.Errorf("ticket %s does not have runtime state", ticketID)
	}
	resumable, ok := s.adapter.(agentrun.ResumableAdapter)
	if !ok {
		return agentrun.PingSummary{}, fmt.Errorf("adapter %s does not support session ping", s.adapter.ID())
	}
	if strings.TrimSpace(status.SessionID) == "" {
		return agentrun.PingSummary{}, fmt.Errorf("ticket %s does not have a persisted codex session", ticketID)
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
		return agentrun.PingSummary{}, fmt.Errorf("no active worktree recorded for %s", ticketID)
	}
	spec := agentrun.RunSpec{
		TicketID:     ticketID,
		Role:         agentrun.RoleImplementer,
		RepoRoot:     s.repoRoot,
		WorktreePath: worktreePath,
		Branch:       branch,
		Prompt:       buildPingPrompt(ticketID, status),
	}
	handle, _, err := resumable.Resume(ctx, status.SessionID, spec)
	if err != nil {
		return agentrun.PingSummary{}, err
	}
	lines, threadID, err := observePing(handle)
	if err != nil {
		return agentrun.PingSummary{}, err
	}
	if threadID != "" {
		status.SessionID = threadID
	}
	if len(lines) > 0 {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		status.SessionMessageCount++
		status.HealthCheckCount++
		status.LastHealthCheckAt = now
		status.LastIntervention = "ping"
		status.LastInterventionAt = now
		status.ConsecutiveNoProgress++
		for _, line := range lines {
			_ = s.runtime.AppendTranscript(ticketID, runruntime.TranscriptEntry{At: now, Text: line})
			status.LastVisibleAt = now
			status.LastVisibleText = line
			if marker, err := agentrun.ParseStatusLine(line); err == nil {
				status.CurrentPhase = marker.Phase
				status.LastMarker = "STATUS"
			}
			if strings.HasPrefix(line, "SUMMARY ") {
				status.LastHealthCheck = line
			}
		}
		status.LastEventAt = now
		_ = s.runtime.WriteStatus(status)
	}
	return agentrun.PingSummary{
		TicketID:  ticketID,
		SessionID: status.SessionID,
		Lines:     lines,
	}, nil
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
		reason := strings.TrimSpace(obs.Result.Reason)
		if reason == "" {
			reason = "reviewer did not emit a REVIEW line"
		}
		return agentrun.ReviewResult{}, reviewerContractError{reason: reason}
	}
	if strings.Contains(obs.Review.RequiredChanges, "malformed REVIEW line") {
		return agentrun.ReviewResult{}, reviewerContractError{reason: obs.Review.RequiredChanges}
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
	return defaultMonitorTimeout
}

func (s *Service) autoResumeLimit() int {
	if s.maxAutoResumes > 0 {
		return s.maxAutoResumes
	}
	return defaultMaxAutoResumes
}

func (s *Service) resumeTimedOutImplementer(ctx context.Context, ticketID, worktreePath, branch string, attempt int) (agentrun.Observation, error) {
	lastObservation := agentrun.Observation{
		Result: agentrun.Result{
			Status:   agentrun.StatusFailed,
			TicketID: ticketID,
			Role:     agentrun.RoleImplementer,
			Reason:   "timed out waiting for additional Codex output",
		},
		TimedOut: true,
	}
	for attempt <= s.autoResumeLimit() {
		if _, err := s.healthCheckTimedOutImplementer(ctx, ticketID); err != nil {
			return agentrun.Observation{}, err
		}
		started, err := s.startAutoResumeAttempt(ctx, ticketID, worktreePath, branch, attempt)
		if err != nil {
			return agentrun.Observation{}, err
		}
		obs, err := s.monitor.Observe(ctx, agentrun.ObservationInput{
			Handle:  started.Handle,
			Record:  started.Record,
			Timeout: s.monitorTimeout(),
		})
		if err != nil {
			return agentrun.Observation{}, err
		}
		lastObservation = obs
		if !obs.TimedOut {
			return obs, nil
		}
		attempt++
	}
	lastObservation.Result = agentrun.Result{
		Status:   agentrun.StatusFailed,
		TicketID: ticketID,
		Role:     agentrun.RoleImplementer,
		Reason:   fmt.Sprintf("run remained inactive after %d health checks; inspect with `docket run-status` and continue with `docket run-resume`", s.autoResumeLimit()+1),
	}
	lastObservation.TimedOut = true
	return lastObservation, nil
}

func (s *Service) healthCheckTimedOutImplementer(ctx context.Context, ticketID string) (agentrun.PingSummary, error) {
	if _, ok := s.adapter.(agentrun.ResumableAdapter); !ok {
		return agentrun.PingSummary{}, nil
	}
	return s.PingTicket(ctx, ticketID)
}

func (s *Service) startAutoResumeAttempt(ctx context.Context, ticketID, worktreePath, branch string, attempt int) (StartedRun, error) {
	if s.runtime == nil {
		return StartedRun{}, fmt.Errorf("runtime store is required")
	}
	status, ok, err := s.runtime.LoadStatus(ticketID)
	if err != nil {
		return StartedRun{}, err
	}
	if !ok {
		return StartedRun{}, fmt.Errorf("ticket %s does not have runtime state", ticketID)
	}
	prompt, err := s.runtime.LoadPrompt(ticketID)
	if err != nil {
		return StartedRun{}, err
	}
	transcript, err := s.runtime.LoadTranscript(ticketID)
	if err != nil {
		return StartedRun{}, err
	}
	tkt, err := s.store.GetTicket(ctx, ticketID)
	if err != nil {
		return StartedRun{}, err
	}
	resumePrompt := buildResumePrompt(prompt, tkt, transcript, status)
	started, err := s.startImplementerContinuation(ctx, ticketID, worktreePath, branch, status.SessionID, resumePrompt)
	if err != nil {
		return StartedRun{}, err
	}
	if s.runtime != nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		status.LastIntervention = "continue_same_thread"
		status.LastInterventionAt = now
		_ = s.runtime.AppendTranscript(ticketID, runruntime.TranscriptEntry{
			At:   now,
			Text: fmt.Sprintf("STATUS ticket=%s phase=healthcheck detail=\"auto-resume attempt %d/%d after inactivity timeout\"", ticketID, attempt, s.autoResumeLimit()),
		})
		_ = s.runtime.WriteStatus(status)
	}
	return started, nil
}

func (s *Service) startImplementerContinuation(ctx context.Context, ticketID, worktreePath, branch, sessionID, prompt string) (StartedRun, error) {
	if resumable, ok := s.adapter.(agentrun.ResumableAdapter); ok && strings.TrimSpace(sessionID) != "" {
		return s.startResumedFollowup(ctx, ticketID, worktreePath, branch, sessionID, agentrun.RoleImplementer, prompt, resumable)
	}
	return s.startFollowup(ctx, ticketID, worktreePath, branch, agentrun.RoleImplementer, prompt, s.adapter)
}

func (s *Service) startResumedFollowup(ctx context.Context, ticketID, worktreePath, branch, sessionID string, role agentrun.Role, prompt string, adapter agentrun.ResumableAdapter) (StartedRun, error) {
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
	handle, record, err := adapter.Resume(ctx, sessionID, spec)
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

func (s *Service) cleanupRuntime(ticketID string) error {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.Cleanup(ticketID)
}

func (s *Service) persistValidationFailureBrief(input agentrun.ValidationInput, validation agentrun.ValidationResult) error {
	if s.runtime == nil || validation.Accepted {
		return nil
	}
	return s.runtime.WriteBrief(runruntime.RunBrief{
		TicketID:         input.TicketID,
		Outcome:          string(agentrun.StatusFailed),
		Summary:          "Managed run failed validation before closeout.",
		SessionID:        strings.TrimSpace(input.SessionID),
		CommitSHA:        strings.TrimSpace(input.Result.CommitSHA),
		Tests:            strings.TrimSpace(input.Result.Tests),
		ValidationErrors: append([]string(nil), validation.Reasons...),
		ResumeNext:       "Inspect the validation failures, repair the worktree, and rerun the ticket.",
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
	})
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
	parts := make([]string, 0, 3)
	if original := strings.TrimSpace(originalPrompt); original != "" {
		parts = append(parts, original)
	}
	parts = append(parts, strings.Join([]string{
		"Previous run hung before completion.",
		"Continue from the current worktree state instead of restarting.",
		"Ticket: " + ticketID,
		"Title: " + title,
		"Last known progress: " + step,
		"Recent visible transcript:",
		strings.Join(lines, "\n"),
	}, "\n"))
	return strings.Join(parts, "\n\n")
}

func buildPingPrompt(ticketID string, status runruntime.StatusSnapshot) string {
	current := strings.TrimSpace(status.CurrentPhase)
	if current == "" {
		current = "unknown"
	}
	last := strings.TrimSpace(status.LastVisibleText)
	if last == "" {
		last = "none"
	}
	return strings.TrimSpace(fmt.Sprintf(
		"Do not run tools or edit files. Return exactly two lines and then stop.\nLine 1 must be: STATUS ticket=%s phase=<single_word_phase>\nLine 2 must be: SUMMARY ticket=%s waiting=<yes|no> note=\"<brief reason>\"\nCurrent phase hint: %s\nLast visible line: %s",
		ticketID,
		ticketID,
		current,
		last,
	))
}

func observePing(handle agentrun.ProcessHandle) ([]string, string, error) {
	if handle == nil {
		return nil, "", fmt.Errorf("process handle is required")
	}
	linesCh := make(chan pingEvent, 64)
	waitCh := make(chan error, 1)
	go scanPingStream(handle.Stdout(), "stdout", linesCh)
	go scanPingStream(handle.Stderr(), "stderr", linesCh)
	go func() {
		waitCh <- handle.Wait()
	}()
	var visible []string
	threadID := ""
	stdoutClosed := false
	stderrClosed := false
	waited := false
	var waitErr error
	for {
		if waited && stdoutClosed && stderrClosed {
			if waitErr != nil {
				return visible, threadID, waitErr
			}
			return visible, threadID, nil
		}
		select {
		case event := <-linesCh:
			if event.done {
				if event.stream == "stdout" {
					stdoutClosed = true
				} else {
					stderrClosed = true
				}
				continue
			}
			if event.stream != "stdout" {
				continue
			}
			if id := pingThreadIDFromLine(event.line); id != "" {
				threadID = id
			}
			visible = append(visible, pingVisibleTextsFromLine(event.line)...)
		case err := <-waitCh:
			waited = true
			waitErr = err
		}
	}
}

type pingEvent struct {
	stream string
	line   string
	done   bool
}

func scanPingStream(r io.Reader, stream string, lines chan<- pingEvent) {
	if r == nil {
		lines <- pingEvent{stream: stream, done: true}
		return
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines <- pingEvent{stream: stream, line: scanner.Text()}
	}
	lines <- pingEvent{stream: stream, done: true}
}

func pingVisibleTextsFromLine(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(line, "STATUS "), strings.HasPrefix(line, "SUMMARY "):
		return []string{line}
	}
	var event struct {
		Type string `json:"type"`
		Item struct {
			Text string `json:"text"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil
	}
	if event.Item.Text == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(event.Item.Text, "\n") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "STATUS ") || strings.HasPrefix(part, "SUMMARY ") {
			out = append(out, part)
		}
	}
	return out
}

func pingThreadIDFromLine(line string) string {
	var event struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &event); err != nil {
		return ""
	}
	if event.Type != "thread.started" {
		return ""
	}
	return strings.TrimSpace(event.ThreadID)
}
