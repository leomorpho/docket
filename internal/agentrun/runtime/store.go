package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	TicketID          string `json:"ticket_id"`
	SessionID         string `json:"session_id,omitempty"`
	Role              string `json:"role,omitempty"`
	PID               int    `json:"pid,omitempty"`
	Active            bool   `json:"active"`
	Hung              bool   `json:"hung,omitempty"`
	LastEventAt       string `json:"last_event_at,omitempty"`
	LastVisibleAt     string `json:"last_visible_at,omitempty"`
	InactivityTimeout string `json:"inactivity_timeout,omitempty"`
	PlannedSteps      int    `json:"planned_steps,omitempty"`
	CurrentStep       int    `json:"current_step,omitempty"`
	CurrentStepTitle  string `json:"current_step_title,omitempty"`
	CurrentPhase      string `json:"current_phase,omitempty"`
	LastMarker        string `json:"last_marker,omitempty"`
	LastVisibleText   string `json:"last_visible_text,omitempty"`
	LastResultStatus  string `json:"last_result_status,omitempty"`
}

type CycleState struct {
	Active           bool   `json:"active"`
	CurrentTicketID  string `json:"current_ticket_id,omitempty"`
	StopAfterCurrent bool   `json:"stop_after_current,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
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
