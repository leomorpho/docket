package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/artifacts"
)

const (
	stdoutFile     = "stdout.jsonl"
	stderrFile     = "stderr.log"
	transcriptFile = "transcript.json"
	statusFile     = "status.json"
	promptFile     = "prompt.txt"
	cycleFile      = "cycle.json"
)

type TranscriptEntry struct {
	At   string `json:"at"`
	Text string `json:"text"`
}

type StatusSnapshot struct {
	TicketID              string `json:"ticket_id"`
	SessionID             string `json:"session_id,omitempty"`
	Role                  string `json:"role,omitempty"`
	PID                   int    `json:"pid,omitempty"`
	Active                bool   `json:"active"`
	Hung                  bool   `json:"hung,omitempty"`
	LastEventAt           string `json:"last_event_at,omitempty"`
	LastVisibleAt         string `json:"last_visible_at,omitempty"`
	InactivityTimeout     string `json:"inactivity_timeout,omitempty"`
	PlannedSteps          int    `json:"planned_steps,omitempty"`
	CurrentStep           int    `json:"current_step,omitempty"`
	CurrentStepTitle      string `json:"current_step_title,omitempty"`
	CurrentPhase          string `json:"current_phase,omitempty"`
	LastMarker            string `json:"last_marker,omitempty"`
	LastVisibleText       string `json:"last_visible_text,omitempty"`
	LastResultStatus      string `json:"last_result_status,omitempty"`
	SessionMessageCount   int    `json:"session_message_count,omitempty"`
	HealthCheckCount      int    `json:"health_check_count,omitempty"`
	LastHealthCheckAt     string `json:"last_health_check_at,omitempty"`
	LastHealthCheck       string `json:"last_health_check,omitempty"`
	LastIntervention      string `json:"last_intervention,omitempty"`
	LastInterventionAt    string `json:"last_intervention_at,omitempty"`
	ConsecutiveNoProgress int    `json:"consecutive_no_progress,omitempty"`
	Warning               string `json:"warning,omitempty"`
}

type CycleState struct {
	Active           bool                `json:"active"`
	CurrentTicketID  string              `json:"current_ticket_id,omitempty"`
	StopAfterCurrent bool                `json:"stop_after_current,omitempty"`
	StartedAt        string              `json:"started_at,omitempty"`
	UpdatedAt        string              `json:"updated_at,omitempty"`
	Completed        []CycleCompletedRun `json:"completed,omitempty"`
}

type CycleCompletedRun struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status,omitempty"`
	Length   string `json:"length,omitempty"`
}

type Store struct {
	repoRoot string
}

func New(repoRoot string) *Store {
	return &Store{repoRoot: repoRoot}
}

func (s *Store) RunsRootDir() string {
	return artifacts.WriteRepoPath(s.repoRoot, artifacts.RepoAgentRunsDir)
}

func (s *Store) RuntimeRootDir() string {
	return filepath.Dir(s.RunsRootDir())
}

func (s *Store) RunDir(ticketID string) string {
	return artifacts.WriteRepoPath(s.repoRoot, artifacts.RepoAgentRunsDir, ticketID)
}

func (s *Store) Init(record agentrun.RunRecord, prompt string, inactivity time.Duration) error {
	dir := s.RunDir(record.TicketID)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, promptFile), []byte(prompt), 0o644); err != nil {
		return err
	}
	return s.WriteStatus(StatusSnapshot{
		TicketID:          record.TicketID,
		SessionID:         record.SessionID,
		Role:              string(record.Role),
		Active:            true,
		InactivityTimeout: inactivity.String(),
	})
}

func (s *Store) AppendStdout(ticketID string, line []byte) error {
	return appendFile(filepath.Join(s.RunDir(ticketID), stdoutFile), line)
}

func (s *Store) AppendStderr(ticketID string, line []byte) error {
	return appendFile(filepath.Join(s.RunDir(ticketID), stderrFile), line)
}

func (s *Store) AppendTranscript(ticketID string, entry TranscriptEntry) error {
	items, err := s.LoadTranscript(ticketID)
	if err != nil {
		return err
	}
	items = append(items, entry)
	return writeJSON(filepath.Join(s.RunDir(ticketID), transcriptFile), items)
}

func (s *Store) LoadTranscript(ticketID string) ([]TranscriptEntry, error) {
	path := filepath.Join(s.RunDir(ticketID), transcriptFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []TranscriptEntry
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) LoadStdoutLines(ticketID string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(s.RunDir(ticketID), stdoutFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func (s *Store) WriteStatus(status StatusSnapshot) error {
	return writeJSON(filepath.Join(s.RunDir(status.TicketID), statusFile), status)
}

func (s *Store) LoadStatus(ticketID string) (StatusSnapshot, bool, error) {
	path := filepath.Join(s.RunDir(ticketID), statusFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusSnapshot{}, false, nil
		}
		return StatusSnapshot{}, false, err
	}
	var status StatusSnapshot
	if err := json.Unmarshal(data, &status); err != nil {
		return StatusSnapshot{}, false, err
	}
	if reconciled, changed := reconcileRuntimeStatus(status); changed {
		status = reconciled
		if err := s.WriteStatus(status); err != nil {
			return StatusSnapshot{}, false, err
		}
	}
	return status, true, nil
}

func (s *Store) LoadPrompt(ticketID string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.RunDir(ticketID), promptFile))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) Cleanup(ticketID string) error {
	return os.RemoveAll(s.RunDir(ticketID))
}

func (s *Store) CleanupStaleRuns() ([]string, error) {
	ticketIDs, err := s.ListRunTicketIDs()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, ticketID := range ticketIDs {
		status, ok, err := s.LoadStatus(ticketID)
		if err != nil {
			if removeErr := s.Cleanup(ticketID); removeErr != nil {
				return removed, removeErr
			}
			removed = append(removed, ticketID)
			continue
		}
		if !ok || !status.Active {
			if err := s.Cleanup(ticketID); err != nil {
				return removed, err
			}
			removed = append(removed, ticketID)
		}
	}
	sort.Strings(removed)
	if len(removed) == 0 {
		return removed, nil
	}
	cycle, ok, err := s.LoadCycleState()
	if err != nil {
		return removed, err
	}
	if ok && cycle.CurrentTicketID != "" && containsString(removed, cycle.CurrentTicketID) {
		if err := s.EndCycle(); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
	}
	return removed, nil
}

func (s *Store) BeginCycle(now time.Time) error {
	return s.WriteCycleState(CycleState{
		Active:    true,
		StartedAt: now.UTC().Format(time.RFC3339Nano),
		UpdatedAt: now.UTC().Format(time.RFC3339Nano),
	})
}

func (s *Store) UpdateCycleCurrent(ticketID string, now time.Time) error {
	state, _, err := s.LoadCycleState()
	if err != nil {
		return err
	}
	state.Active = true
	state.CurrentTicketID = ticketID
	if state.StartedAt == "" {
		state.StartedAt = now.UTC().Format(time.RFC3339Nano)
	}
	state.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	return s.WriteCycleState(state)
}

func (s *Store) RequestStopAfterCurrent(now time.Time) error {
	state, _, err := s.LoadCycleState()
	if err != nil {
		return err
	}
	state.StopAfterCurrent = true
	state.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	return s.WriteCycleState(state)
}

func (s *Store) AppendCycleCompleted(ticketID, status, length string, now time.Time) error {
	state, _, err := s.LoadCycleState()
	if err != nil {
		return err
	}
	state.Active = true
	state.Completed = append(state.Completed, CycleCompletedRun{
		TicketID: ticketID,
		Status:   status,
		Length:   length,
	})
	state.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	return s.WriteCycleState(state)
}

func (s *Store) StopAfterCurrentRequested() (bool, error) {
	state, ok, err := s.LoadCycleState()
	if err != nil || !ok {
		return false, err
	}
	return state.StopAfterCurrent, nil
}

func (s *Store) WriteCycleState(state CycleState) error {
	return writeJSON(filepath.Join(s.RuntimeRootDir(), cycleFile), state)
}

func (s *Store) LoadCycleState() (CycleState, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.RuntimeRootDir(), cycleFile))
	if err != nil {
		if os.IsNotExist(err) {
			return CycleState{}, false, nil
		}
		return CycleState{}, false, err
	}
	var state CycleState
	if err := json.Unmarshal(data, &state); err != nil {
		return CycleState{}, false, err
	}
	return state, true, nil
}

func (s *Store) EndCycle() error {
	return os.Remove(filepath.Join(s.RuntimeRootDir(), cycleFile))
}

func (s *Store) HealRuntimeState(now time.Time) ([]string, error) {
	var warnings []string
	cycle, ok, err := s.LoadCycleState()
	if err != nil {
		if removeErr := s.EndCycle(); removeErr != nil && !os.IsNotExist(removeErr) {
			return warnings, removeErr
		}
		return append(warnings, "cycle state was unreadable and has been cleared"), nil
	}
	if !ok {
		return warnings, nil
	}
	if strings.TrimSpace(cycle.CurrentTicketID) == "" {
		return warnings, nil
	}
	status, statusOK, err := s.LoadStatus(cycle.CurrentTicketID)
	if err != nil {
		if endErr := s.EndCycle(); endErr != nil && !os.IsNotExist(endErr) {
			return warnings, endErr
		}
		return append(warnings, fmt.Sprintf("cleared stale cycle for %s after status read error", cycle.CurrentTicketID)), nil
	}
	if !statusOK || !status.Active {
		if endErr := s.EndCycle(); endErr != nil && !os.IsNotExist(endErr) {
			return warnings, endErr
		}
		return append(warnings, fmt.Sprintf("cleared stale cycle for %s", cycle.CurrentTicketID)), nil
	}
	return warnings, nil
}

func (s *Store) HardStopRun(ticketID string, now time.Time) error {
	status, ok, err := s.LoadStatus(ticketID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("ticket %s does not have runtime state", ticketID)
	}
	if status.PID > 0 && processAlive(status.PID) {
		_ = syscall.Kill(status.PID, syscall.SIGTERM)
		time.Sleep(150 * time.Millisecond)
		if processAlive(status.PID) {
			_ = syscall.Kill(status.PID, syscall.SIGKILL)
		}
	}
	status.Active = false
	status.Hung = false
	status.LastEventAt = now.UTC().Format(time.RFC3339Nano)
	status.LastVisibleAt = status.LastEventAt
	status.LastVisibleText = "Operator requested hard stop"
	status.LastResultStatus = "stopped"
	status.Warning = ""
	if err := s.WriteStatus(status); err != nil {
		return err
	}
	if cycle, ok, err := s.LoadCycleState(); err == nil && ok {
		cycle.StopAfterCurrent = true
		cycle.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
		_ = s.WriteCycleState(cycle)
	}
	return nil
}

func (s *Store) ListRunTicketIDs() ([]string, error) {
	entries, err := os.ReadDir(s.RunsRootDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

func appendFile(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

func reconcileRuntimeStatus(status StatusSnapshot) (StatusSnapshot, bool) {
	if !status.Active || status.PID <= 0 {
		return status, false
	}
	if processAlive(status.PID) {
		return status, false
	}
	status.Active = false
	status.Hung = true
	if status.LastResultStatus == "" {
		status.LastResultStatus = string(agentrun.StatusFailed)
	}
	return status, true
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, syscall.Signal(0))
	if err == nil {
		return true
	}
	return !errors.Is(err, syscall.ESRCH)
}

func writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
