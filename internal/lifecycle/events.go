package lifecycle

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
)

const (
	SchemaVersionV1 = "docket.lifecycle/v1"

	EventRunStart        = "run.start"
	EventPhaseEnd        = "phase.end"
	EventRunEnd          = "run.end"
	EventToolFailure     = "tool.failure"
	EventProofMutation   = "proof.mutation"
	EventStateTransition = "state.transition"

	StatusOK     = "ok"
	StatusFailed = "failed"

	CodeRequired           = "required"
	CodeTypeMismatch       = "type_mismatch"
	CodeInvalidValue       = "invalid_value"
	CodeUnsupportedVersion = "unsupported_version"
)

var runSequence uint64

type Event struct {
	Version   string         `json:"version"`
	Type      string         `json:"type"`
	EmittedAt string         `json:"emitted_at"`
	Payload   map[string]any `json:"payload"`
}

type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationReport struct {
	SchemaVersion string            `json:"schema_version"`
	Errors        []ValidationError `json:"errors,omitempty"`
}

func (r ValidationReport) Valid() bool {
	return len(r.Errors) == 0
}

type validator struct {
	errors []ValidationError
}

func (v *validator) add(path, code, message string) {
	v.errors = append(v.errors, ValidationError{
		Path:    path,
		Code:    code,
		Message: message,
	})
}

func (v *validator) sortedErrors() []ValidationError {
	sort.SliceStable(v.errors, func(i, j int) bool {
		if v.errors[i].Path == v.errors[j].Path {
			return v.errors[i].Code < v.errors[j].Code
		}
		return v.errors[i].Path < v.errors[j].Path
	})
	return v.errors
}

func ValidateEvent(event Event) ValidationReport {
	v := &validator{}
	report := ValidationReport{SchemaVersion: SchemaVersionV1}

	if strings.TrimSpace(event.Version) == "" {
		v.add("version", CodeRequired, "field is required")
	} else if strings.TrimSpace(event.Version) != SchemaVersionV1 {
		v.add("version", CodeUnsupportedVersion, fmt.Sprintf("unsupported version %q", event.Version))
	}
	if strings.TrimSpace(event.Type) == "" {
		v.add("type", CodeRequired, "field is required")
	}
	if strings.TrimSpace(event.EmittedAt) == "" {
		v.add("emitted_at", CodeRequired, "field is required")
	} else if _, err := time.Parse(time.RFC3339Nano, event.EmittedAt); err != nil {
		v.add("emitted_at", CodeInvalidValue, "must be RFC3339 timestamp")
	}
	if event.Payload == nil {
		v.add("payload", CodeRequired, "field is required")
		report.Errors = v.sortedErrors()
		return report
	}

	switch event.Type {
	case EventRunStart:
		requireString(v, event.Payload, "payload.run_id", "run_id")
		requireString(v, event.Payload, "payload.command", "command")
		requireString(v, event.Payload, "payload.repo_root", "repo_root")
	case EventPhaseEnd:
		requireString(v, event.Payload, "payload.run_id", "run_id")
		requireString(v, event.Payload, "payload.command", "command")
		requireString(v, event.Payload, "payload.phase", "phase")
		status := requireString(v, event.Payload, "payload.status", "status")
		if status != "" && status != StatusOK && status != StatusFailed {
			v.add("payload.status", CodeInvalidValue, "must be ok or failed")
		}
	case EventRunEnd:
		requireString(v, event.Payload, "payload.run_id", "run_id")
		requireString(v, event.Payload, "payload.command", "command")
		status := requireString(v, event.Payload, "payload.status", "status")
		if status != "" && status != StatusOK && status != StatusFailed {
			v.add("payload.status", CodeInvalidValue, "must be ok or failed")
		}
	case EventToolFailure:
		requireString(v, event.Payload, "payload.run_id", "run_id")
		requireString(v, event.Payload, "payload.command", "command")
		requireString(v, event.Payload, "payload.phase", "phase")
		requireString(v, event.Payload, "payload.tool", "tool")
		requireString(v, event.Payload, "payload.error", "error")
	case EventProofMutation:
		requireString(v, event.Payload, "payload.command", "command")
		requireString(v, event.Payload, "payload.ticket_id", "ticket_id")
		requireString(v, event.Payload, "payload.proof_id", "proof_id")
		requireString(v, event.Payload, "payload.blob_sha256", "blob_sha256")
		requireString(v, event.Payload, "payload.actor", "actor")
		action := requireString(v, event.Payload, "payload.action", "action")
		if action != "" && action != "add" && action != "remove" {
			v.add("payload.action", CodeInvalidValue, "must be add or remove")
		}
	case EventStateTransition:
		requireString(v, event.Payload, "payload.command", "command")
		requireString(v, event.Payload, "payload.ticket_id", "ticket_id")
		requireString(v, event.Payload, "payload.actor", "actor")
		requireString(v, event.Payload, "payload.from_state", "from_state")
		requireString(v, event.Payload, "payload.to_state", "to_state")
		requireString(v, event.Payload, "payload.reason", "reason")
	default:
		v.add("type", CodeInvalidValue, "must be run.start,phase.end,run.end,tool.failure,proof.mutation,state.transition")
	}

	report.Errors = v.sortedErrors()
	return report
}

func requireString(v *validator, payload map[string]any, path, key string) string {
	val, ok := payload[key]
	if !ok {
		v.add(path, CodeRequired, "field is required")
		return ""
	}
	s, ok := val.(string)
	if !ok {
		v.add(path, CodeTypeMismatch, "must be a string")
		return ""
	}
	if strings.TrimSpace(s) == "" {
		v.add(path, CodeRequired, "field cannot be empty")
		return ""
	}
	return s
}

func LogPath(repoRoot string) string {
	return artifacts.WriteRepoPath(repoRoot, artifacts.RepoLifecycleEvents)
}

func Append(repoRoot string, event Event) error {
	if strings.TrimSpace(event.Version) == "" {
		event.Version = SchemaVersionV1
	}
	if strings.TrimSpace(event.EmittedAt) == "" {
		event.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	report := ValidateEvent(event)
	if !report.Valid() {
		return fmt.Errorf("invalid lifecycle event: %#v", report.Errors)
	}

	path := LogPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func Load(repoRoot string) ([]Event, error) {
	path := artifacts.ReadRepoPath(repoRoot, artifacts.RepoLifecycleEvents)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

type RunInput struct {
	RepoRoot string
	Command  string
	TicketID string
	Actor    string
	RunID    string
	Now      func() time.Time
}

type Recorder struct {
	repoRoot string
	runID    string
	command  string
	ticketID string
	actor    string
	now      func() time.Time
	ended    bool
}

func StartRun(input RunInput) (*Recorder, error) {
	if strings.TrimSpace(input.RepoRoot) == "" {
		return nil, fmt.Errorf("repo root is required")
	}
	if strings.TrimSpace(input.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	now := input.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = defaultRunID(now())
	}
	rec := &Recorder{
		repoRoot: input.RepoRoot,
		runID:    runID,
		command:  input.Command,
		ticketID: strings.TrimSpace(input.TicketID),
		actor:    strings.TrimSpace(input.Actor),
		now:      now,
	}
	if err := rec.append(EventRunStart, map[string]any{
		"run_id":    rec.runID,
		"command":   rec.command,
		"repo_root": rec.repoRoot,
		"ticket_id": rec.ticketID,
		"actor":     rec.actor,
	}); err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Recorder) PhaseEnd(phase, status string) error {
	if r == nil {
		return nil
	}
	return r.append(EventPhaseEnd, map[string]any{
		"run_id":    r.runID,
		"command":   r.command,
		"ticket_id": r.ticketID,
		"phase":     strings.TrimSpace(phase),
		"status":    strings.TrimSpace(status),
	})
}

func (r *Recorder) ToolFailure(phase, tool string, failure error) error {
	if r == nil {
		return nil
	}
	msg := ""
	if failure != nil {
		msg = strings.TrimSpace(failure.Error())
	}
	return r.append(EventToolFailure, map[string]any{
		"run_id":    r.runID,
		"command":   r.command,
		"ticket_id": r.ticketID,
		"phase":     strings.TrimSpace(phase),
		"tool":      strings.TrimSpace(tool),
		"error":     msg,
	})
}

func (r *Recorder) End(status string) error {
	if r == nil || r.ended {
		return nil
	}
	r.ended = true
	return r.append(EventRunEnd, map[string]any{
		"run_id":    r.runID,
		"command":   r.command,
		"ticket_id": r.ticketID,
		"status":    strings.TrimSpace(status),
	})
}

func (r *Recorder) append(eventType string, payload map[string]any) error {
	event := Event{
		Version:   SchemaVersionV1,
		Type:      eventType,
		EmittedAt: r.now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	}
	return Append(r.repoRoot, event)
}

func defaultRunID(now time.Time) string {
	seq := atomic.AddUint64(&runSequence, 1)
	return fmt.Sprintf("run-%d-%d", now.UnixNano(), seq)
}
