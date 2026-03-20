package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
)

type Dependencies struct {
	Runtime *runruntime.Store
	Now     func() time.Time
}

type Observer struct {
	runtime *runruntime.Store
	now     func() time.Time
}

func New(deps ...Dependencies) *Observer {
	if len(deps) == 0 {
		return &Observer{now: time.Now}
	}
	now := deps[0].Now
	if now == nil {
		now = time.Now
	}
	return &Observer{
		runtime: deps[0].Runtime,
		now:     now,
	}
}

type lineEvent struct {
	stream string
	line   string
}

func (o *Observer) Observe(ctx context.Context, input agentrun.ObservationInput) (agentrun.Observation, error) {
	if input.Handle == nil {
		return agentrun.Observation{}, fmt.Errorf("process handle is required")
	}

	lines := make(chan lineEvent, 64)
	stdoutDone := make(chan string, 1)
	stderrDone := make(chan string, 1)
	waitCh := make(chan error, 1)

	go scanStream(input.Handle.Stdout(), "stdout", lines, stdoutDone)
	go scanStream(input.Handle.Stderr(), "stderr", lines, stderrDone)
	go func() {
		waitCh <- input.Handle.Wait()
	}()

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if err := o.writeInitialStatus(input, timeout); err != nil {
		return agentrun.Observation{}, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var stdoutLines, stderrLines []string
	status := runruntime.StatusSnapshot{
		TicketID:          input.Record.TicketID,
		SessionID:         input.Record.SessionID,
		Role:              string(input.Record.Role),
		PID:               input.Handle.PID(),
		Active:            true,
		InactivityTimeout: timeout.String(),
	}

	var waitErr error
	waited := false
	stdoutClosed := false
	stderrClosed := false
	stdoutProcessed := 0
	stderrProcessed := 0
	for {
		if waited && stdoutClosed && stderrClosed {
			return o.finalizeObservation(input, status, waitErr, stdoutLines, stderrLines)
		}
		select {
		case <-ctx.Done():
			_ = input.Handle.Kill()
			status.Active = false
			status.Hung = true
			status.LastResultStatus = string(agentrun.StatusFailed)
			_ = o.writeStatus(status)
			return agentrun.Observation{
				Result:   failureResult(input.Record, fmt.Sprintf("run cancelled: %v", ctx.Err())),
				TimedOut: true,
			}, nil
		case <-timer.C:
			_ = input.Handle.Kill()
			status.Active = false
			status.Hung = true
			status.LastResultStatus = "hung"
			_ = o.writeStatus(status)
			return agentrun.Observation{
				Result:   failureResult(input.Record, "timed out waiting for additional Codex output"),
				TimedOut: true,
			}, nil
		case event := <-lines:
			if event.stream == "stdout" {
				stdoutLines = append(stdoutLines, event.line)
				stdoutProcessed++
			} else {
				stderrLines = append(stderrLines, event.line)
				stderrProcessed++
			}
			o.applyLine(input.Record.TicketID, event.stream, event.line, &status)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		case combined := <-stdoutDone:
			stdoutClosed = true
			if combined != "" {
				all := splitNonEmptyLines(combined)
				stdoutLines = all
				for _, line := range all[stdoutProcessed:] {
					o.applyLine(input.Record.TicketID, "stdout", line, &status)
				}
			}
		case combined := <-stderrDone:
			stderrClosed = true
			if combined != "" {
				all := splitNonEmptyLines(combined)
				stderrLines = all
				for _, line := range all[stderrProcessed:] {
					o.applyLine(input.Record.TicketID, "stderr", line, &status)
				}
			}
		case err := <-waitCh:
			waited = true
			waitErr = err
		}
	}
}

func (o *Observer) applyLine(ticketID, stream, line string, status *runruntime.StatusSnapshot) {
	now := o.now().UTC()
	status.LastEventAt = now.Format(time.RFC3339Nano)
	if stream == "stdout" {
		if o.runtime != nil {
			_ = o.runtime.AppendStdout(ticketID, []byte(line+"\n"))
		}
	} else if o.runtime != nil {
		_ = o.runtime.AppendStderr(ticketID, []byte(line+"\n"))
	}
	for _, visible := range visibleTextsFromLine(line) {
		status.LastVisibleAt = now.Format(time.RFC3339Nano)
		status.LastVisibleText = visible
		o.updateProgressStatus(status, visible)
		if o.runtime != nil {
			_ = o.runtime.AppendTranscript(ticketID, runruntime.TranscriptEntry{
				At:   now.Format(time.RFC3339Nano),
				Text: visible,
			})
		}
	}
	_ = o.writeStatus(*status)
}

func (o *Observer) writeInitialStatus(input agentrun.ObservationInput, timeout time.Duration) error {
	if o.runtime == nil {
		return nil
	}
	return o.runtime.WriteStatus(runruntime.StatusSnapshot{
		TicketID:          input.Record.TicketID,
		SessionID:         input.Record.SessionID,
		Role:              string(input.Record.Role),
		PID:               input.Handle.PID(),
		Active:            true,
		InactivityTimeout: timeout.String(),
	})
}

func (o *Observer) writeStatus(status runruntime.StatusSnapshot) error {
	if o.runtime == nil {
		return nil
	}
	return o.runtime.WriteStatus(status)
}

func (o *Observer) finalizeObservation(input agentrun.ObservationInput, status runruntime.StatusSnapshot, waitErr error, stdoutLines, stderrLines []string) (agentrun.Observation, error) {
	status.Active = false
	defer func() { _ = o.writeStatus(status) }()

	if review, ok := parseOutputReview(stdoutLines, stderrLines); ok {
		return agentrun.Observation{Review: &review}, nil
	}
	if result, ok := parseOutputResult(stdoutLines, stderrLines); ok {
		status.LastResultStatus = string(result.Status)
		if waitErr == nil {
			return agentrun.Observation{Result: result}, nil
		}
		return agentrun.Observation{
			Result: failureResult(input.Record, fmt.Sprintf("process exited after RESULT line: %v", waitErr)),
		}, nil
	}
	if malformed := malformedReviewReason(stdoutLines, stderrLines); malformed != "" && input.Record.Role == agentrun.RoleReviewer {
		return agentrun.Observation{
			Review: &agentrun.ReviewResult{
				Status:          agentrun.ReviewChangesRequired,
				TicketID:        input.Record.TicketID,
				Role:            agentrun.RoleReviewer,
				RequiredChanges: malformed,
			},
		}, nil
	}
	if malformed := malformedResultReason(stdoutLines, stderrLines); malformed != "" {
		status.LastResultStatus = string(agentrun.StatusFailed)
		return agentrun.Observation{
			Result: failureResult(input.Record, malformed),
		}, nil
	}
	status.LastResultStatus = string(agentrun.StatusFailed)
	if waitErr != nil {
		return agentrun.Observation{
			Result: failureResult(input.Record, fmt.Sprintf("process exited without RESULT line: %v", waitErr)),
		}, nil
	}
	return agentrun.Observation{
		Result: failureResult(input.Record, "process exited without RESULT line"),
	}, nil
}

func (o *Observer) updateProgressStatus(status *runruntime.StatusSnapshot, visible string) {
	switch {
	case strings.HasPrefix(visible, "PLAN "):
		if plan, err := agentrun.ParsePlanLine(visible); err == nil {
			status.PlannedSteps = plan.Steps
			status.LastMarker = "PLAN"
		}
	case strings.HasPrefix(visible, "STEP "):
		if step, err := agentrun.ParseStepLine(visible); err == nil {
			status.CurrentStep = step.Index
			status.CurrentStepTitle = step.Title
			status.LastMarker = "STEP"
		}
	case strings.HasPrefix(visible, "STATUS "):
		if marker, err := agentrun.ParseStatusLine(visible); err == nil {
			status.CurrentPhase = marker.Phase
			status.LastMarker = "STATUS"
		}
	case strings.HasPrefix(visible, "RESULT "):
		status.LastMarker = "RESULT"
	case strings.HasPrefix(visible, "REVIEW "):
		status.LastMarker = "REVIEW"
	}
}

func scanStream(r io.Reader, stream string, lines chan<- lineEvent, done chan<- string) {
	if r == nil {
		done <- ""
		return
	}
	var collected []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		collected = append(collected, line)
		lines <- lineEvent{stream: stream, line: line}
	}
	done <- strings.Join(collected, "\n")
}

func splitNonEmptyLines(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseOutputResult(stdoutLines, stderrLines []string) (agentrun.Result, bool) {
	for _, line := range append(append([]string(nil), stdoutLines...), stderrLines...) {
		text := resultTextFromLine(line)
		if text == "" {
			continue
		}
		result, err := agentrun.ParseResultLine(text)
		if err == nil {
			return result, true
		}
	}
	return agentrun.Result{}, false
}

func malformedResultReason(stdoutLines, stderrLines []string) string {
	for _, line := range append(append([]string(nil), stdoutLines...), stderrLines...) {
		text := resultTextFromLine(line)
		if text == "" {
			continue
		}
		if _, err := agentrun.ParseResultLine(text); err != nil {
			return fmt.Sprintf("malformed RESULT line: %v", err)
		}
	}
	return ""
}

func parseOutputReview(stdoutLines, stderrLines []string) (agentrun.ReviewResult, bool) {
	for _, line := range append(append([]string(nil), stdoutLines...), stderrLines...) {
		text := reviewTextFromLine(line)
		if text == "" {
			continue
		}
		result, err := agentrun.ParseReviewLine(text)
		if err == nil {
			return result, true
		}
	}
	return agentrun.ReviewResult{}, false
}

func malformedReviewReason(stdoutLines, stderrLines []string) string {
	for _, line := range append(append([]string(nil), stdoutLines...), stderrLines...) {
		text := reviewTextFromLine(line)
		if text == "" {
			continue
		}
		if _, err := agentrun.ParseReviewLine(text); err != nil {
			return fmt.Sprintf("malformed REVIEW line: %v", err)
		}
	}
	return ""
}

func failureResult(record agentrun.RunRecord, reason string) agentrun.Result {
	return agentrun.Result{
		Status:   agentrun.StatusFailed,
		TicketID: record.TicketID,
		Role:     record.Role,
		Reason:   reason,
	}
}

type codexJSONItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexJSONEvent struct {
	Type string        `json:"type"`
	Item codexJSONItem `json:"item"`
}

func visibleTextsFromLine(line string) []string {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "PLAN "),
		strings.HasPrefix(line, "STEP "),
		strings.HasPrefix(line, "STATUS "),
		strings.HasPrefix(line, "RESULT "),
		strings.HasPrefix(line, "REVIEW "):
		return []string{line}
	}
	var event codexJSONEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil
	}
	text := strings.TrimSpace(event.Item.Text)
	if text == "" {
		return nil
	}
	out := make([]string, 0)
	for _, part := range strings.Split(text, "\n") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func resultTextFromLine(line string) string {
	for _, text := range visibleTextsFromLine(line) {
		if strings.HasPrefix(text, "RESULT ") {
			return text
		}
	}
	return ""
}

func reviewTextFromLine(line string) string {
	for _, text := range visibleTextsFromLine(line) {
		if strings.HasPrefix(text, "REVIEW ") {
			return text
		}
	}
	return ""
}
