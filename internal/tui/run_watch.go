package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
)

type watchMode string

const (
	watchModeSummary watchMode = "summary"
	watchModeLog     watchMode = "log"
)

type runWatchSnapshot struct {
	cycle      runruntime.CycleState
	cycleOK    bool
	ticketID   string
	status     runruntime.StatusSnapshot
	statusOK   bool
	transcript []runruntime.TranscriptEntry
}

type runWatchLoadedMsg struct {
	snapshot runWatchSnapshot
	err      error
}

type runWatchDoneMsg struct{}
type runWatchTickMsg struct{}

type RunWatchModel struct {
	repoRoot       string
	store          *runruntime.Store
	focusTicketID  string
	mode           watchMode
	width          int
	height         int
	statusMessage  string
	snapshot       runWatchSnapshot
	doneCh         <-chan struct{}
	quitOnDone     bool
	showDoneNotice bool
}

func NewRunWatchModel(repoRoot string, focusTicketID string, doneCh <-chan struct{}, quitOnDone bool) RunWatchModel {
	return RunWatchModel{
		repoRoot:      repoRoot,
		store:         runruntime.New(repoRoot),
		focusTicketID: focusTicketID,
		mode:          watchModeSummary,
		doneCh:        doneCh,
		quitOnDone:    quitOnDone,
	}
}

func (m RunWatchModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.loadCmd(),
		m.tickCmd(),
	}
	return tea.Batch(cmds...)
}

func (m RunWatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case runWatchLoadedMsg:
		if msg.err != nil {
			m.statusMessage = "watch refresh failed: " + msg.err.Error()
			return m, m.reloadTick()
		}
		m.snapshot = msg.snapshot
		if m.snapshot.ticketID == "" {
			m.statusMessage = "waiting for managed run"
		} else if m.snapshot.cycle.StopAfterCurrent {
			m.statusMessage = "stop requested after current ticket"
		} else {
			m.statusMessage = "watching managed run"
		}
		return m, m.tickCmd()
	case runWatchTickMsg:
		return m, tea.Batch(m.loadCmd(), m.tickCmd())
	case runWatchDoneMsg:
		if m.doneCh != nil {
			select {
			case <-m.doneCh:
				m.showDoneNotice = true
				if m.quitOnDone {
					return m, tea.Quit
				}
			default:
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		case "tab", "l":
			if m.mode == watchModeSummary {
				m.mode = watchModeLog
			} else {
				m.mode = watchModeSummary
			}
			return m, nil
		case "s":
			if err := m.store.RequestStopAfterCurrent(time.Now()); err != nil {
				m.statusMessage = "failed to request stop: " + err.Error()
				return m, nil
			}
			m.statusMessage = "stop requested after current ticket"
			m.snapshot.cycle.StopAfterCurrent = true
			return m, nil
		case "r":
			return m, m.loadCmd()
		}
	}
	return m, nil
}

func (m RunWatchModel) View() string {
	header := lipgloss.NewStyle().Bold(true).Render("Managed Run Watch")
	repoLine := fmt.Sprintf("repo: %s", filepath.Base(m.repoRoot))
	modeLine := fmt.Sprintf("mode: %s", m.mode)
	stopLine := "stop: running"
	if m.snapshot.cycle.StopAfterCurrent {
		stopLine = "stop: after current ticket"
	}
	current := "ticket: (none)"
	if m.snapshot.ticketID != "" {
		current = fmt.Sprintf("ticket: %s", m.snapshot.ticketID)
	}
	lines := []string{header, repoLine + " | " + current + " | " + modeLine + " | " + stopLine}
	if m.snapshot.statusOK {
		statusLine := fmt.Sprintf("active=%t hung=%t", m.snapshot.status.Active, m.snapshot.status.Hung)
		if m.snapshot.status.CurrentStepTitle != "" {
			statusLine += fmt.Sprintf(" | step %d/%d %s", m.snapshot.status.CurrentStep, m.snapshot.status.PlannedSteps, m.snapshot.status.CurrentStepTitle)
		}
		if m.snapshot.status.CurrentPhase != "" {
			statusLine += fmt.Sprintf(" | phase=%s", m.snapshot.status.CurrentPhase)
		}
		lines = append(lines, statusLine)
	}
	lines = append(lines, "")
	if m.mode == watchModeSummary {
		lines = append(lines, m.renderSummaryBody())
	} else {
		lines = append(lines, m.renderLogBody())
	}
	lines = append(lines, "")
	if m.showDoneNotice {
		lines = append(lines, "run finished")
	}
	if m.statusMessage != "" {
		lines = append(lines, m.statusMessage)
	}
	lines = append(lines, "keys: "+runWatchKeyLegend())
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) renderSummaryBody() string {
	if m.snapshot.ticketID == "" {
		return "No active managed run detected."
	}
	body := []string{}
	if m.snapshot.status.LastVisibleText != "" {
		body = append(body, "Last visible:")
		body = append(body, "  "+m.snapshot.status.LastVisibleText)
	}
	if m.snapshot.status.LastEventAt != "" {
		body = append(body, "Last event: "+m.snapshot.status.LastEventAt)
	}
	transcript := m.snapshot.transcript
	if len(transcript) > 5 {
		transcript = transcript[len(transcript)-5:]
	}
	if len(transcript) > 0 {
		body = append(body, "Recent transcript:")
		for _, entry := range transcript {
			body = append(body, "  "+entry.Text)
		}
	}
	return strings.Join(body, "\n")
}

func (m RunWatchModel) renderLogBody() string {
	if len(m.snapshot.transcript) == 0 {
		return "No visible transcript yet."
	}
	lines := make([]string, 0, len(m.snapshot.transcript))
	for _, entry := range m.snapshot.transcript {
		lines = append(lines, entry.Text)
	}
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) reloadTick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return runWatchTickMsg{}
	})
}

func (m RunWatchModel) tickCmd() tea.Cmd {
	return m.reloadTick()
}

func (m RunWatchModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		snapshot, err := loadRunWatchSnapshot(m.store, m.focusTicketID)
		return runWatchLoadedMsg{snapshot: snapshot, err: err}
	}
}

func loadRunWatchSnapshot(store *runruntime.Store, focusTicketID string) (runWatchSnapshot, error) {
	var snapshot runWatchSnapshot
	if store == nil {
		return snapshot, nil
	}
	cycle, ok, err := store.LoadCycleState()
	if err != nil {
		return snapshot, err
	}
	snapshot.cycle = cycle
	snapshot.cycleOK = ok
	ticketID, status, statusOK, err := selectWatchedTicket(store, focusTicketID, cycle)
	if err != nil {
		return snapshot, err
	}
	snapshot.ticketID = ticketID
	snapshot.status = status
	snapshot.statusOK = statusOK
	if ticketID != "" {
		transcript, err := store.LoadTranscript(ticketID)
		if err != nil {
			return snapshot, err
		}
		snapshot.transcript = transcript
	}
	return snapshot, nil
}

func selectWatchedTicket(store *runruntime.Store, focusTicketID string, cycle runruntime.CycleState) (string, runruntime.StatusSnapshot, bool, error) {
	if focusTicketID != "" {
		status, ok, err := store.LoadStatus(focusTicketID)
		return focusTicketID, status, ok, err
	}
	if cycle.CurrentTicketID != "" {
		status, ok, err := store.LoadStatus(cycle.CurrentTicketID)
		if err != nil {
			return "", runruntime.StatusSnapshot{}, false, err
		}
		if ok {
			return cycle.CurrentTicketID, status, ok, nil
		}
	}
	ticketIDs, err := store.ListRunTicketIDs()
	if err != nil {
		return "", runruntime.StatusSnapshot{}, false, err
	}
	type candidate struct {
		ticketID string
		status   runruntime.StatusSnapshot
	}
	candidates := make([]candidate, 0, len(ticketIDs))
	for _, ticketID := range ticketIDs {
		status, ok, err := store.LoadStatus(ticketID)
		if err != nil {
			return "", runruntime.StatusSnapshot{}, false, err
		}
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{ticketID: ticketID, status: status})
	}
	if len(candidates) == 0 {
		return "", runruntime.StatusSnapshot{}, false, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].status.Active != candidates[j].status.Active {
			return candidates[i].status.Active
		}
		return candidates[i].status.LastEventAt > candidates[j].status.LastEventAt
	})
	best := candidates[0]
	return best.ticketID, best.status, true, nil
}

func RunWatchKeys() []string {
	return slices.Clone([]string{"l", "tab", "s", "r", "q"})
}

func runWatchKeyLegend() string {
	return "l/tab toggle logs | s stop after current | r refresh | q quit"
}
