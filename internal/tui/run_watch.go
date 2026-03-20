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

type launchMode string

const (
	launchModeMenu  launchMode = "menu"
	launchModeWatch launchMode = "watch"
)

type RunWatchLaunchOption struct {
	ID          string
	Label       string
	Description string
	Start       func() error
}

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
type runWatchLaunchResultMsg struct {
	err error
}

type RunWatchModel struct {
	repoRoot       string
	store          *runruntime.Store
	focusTicketID  string
	launchMode     launchMode
	launchOptions  []RunWatchLaunchOption
	selectedOption int
	launching      bool
	mode           watchMode
	width          int
	height         int
	statusMessage  string
	snapshot       runWatchSnapshot
	doneCh         <-chan struct{}
	quitOnDone     bool
	showDoneNotice bool
}

func NewRunWatchModel(repoRoot string, focusTicketID string, doneCh <-chan struct{}, quitOnDone bool, launchOptions []RunWatchLaunchOption) RunWatchModel {
	model := RunWatchModel{
		repoRoot:      repoRoot,
		store:         runruntime.New(repoRoot),
		focusTicketID: focusTicketID,
		mode:          watchModeSummary,
		doneCh:        doneCh,
		quitOnDone:    quitOnDone,
	}
	if len(launchOptions) > 0 {
		model.launchMode = launchModeMenu
		model.launchOptions = slices.Clone(launchOptions)
	} else {
		model.launchMode = launchModeWatch
	}
	return model
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
	case runWatchLaunchResultMsg:
		m.launching = false
		if msg.err != nil {
			m.statusMessage = "launch failed: " + msg.err.Error()
			m.launchMode = launchModeMenu
			return m, nil
		}
		m.launchMode = launchModeWatch
		m.statusMessage = "watching managed run"
		return m, nil
	case runWatchDoneMsg:
		if m.doneCh != nil {
			select {
			case <-m.doneCh:
				m.launching = false
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
		case "up", "k":
			if m.launchMode == launchModeMenu && len(m.launchOptions) > 0 {
				m.selectedOption--
				if m.selectedOption < 0 {
					m.selectedOption = len(m.launchOptions) - 1
				}
			}
			return m, nil
		case "down", "j":
			if m.launchMode == launchModeMenu && len(m.launchOptions) > 0 {
				m.selectedOption = (m.selectedOption + 1) % len(m.launchOptions)
			}
			return m, nil
		case "enter":
			if m.launchMode == launchModeMenu {
				return m.startSelectedOption()
			}
			return m, nil
		case "tab", "l":
			if m.launchMode != launchModeWatch {
				return m, nil
			}
			if m.mode == watchModeSummary {
				m.mode = watchModeLog
			} else {
				m.mode = watchModeSummary
			}
			return m, nil
		case "s":
			if m.launchMode != launchModeWatch {
				return m, nil
			}
			if err := m.store.RequestStopAfterCurrent(time.Now()); err != nil {
				m.statusMessage = "failed to request stop: " + err.Error()
				return m, nil
			}
			m.statusMessage = "stop requested after current ticket"
			m.snapshot.cycle.StopAfterCurrent = true
			return m, nil
		case "r":
			return m, m.loadCmd()
		case "m", "esc":
			if len(m.launchOptions) > 0 && !m.launching {
				m.launchMode = launchModeMenu
				m.statusMessage = "choose a managed run mode"
			}
			return m, nil
		}
	}
	return m, nil
}

func (m RunWatchModel) View() string {
	header := lipgloss.NewStyle().Bold(true).Render("Managed Run")
	repoLine := fmt.Sprintf("repo: %s", filepath.Base(m.repoRoot))
	if m.launchMode == launchModeMenu {
		lines := []string{header, repoLine, "", m.renderMenuBody(), "", "keys: " + menuKeyLegend()}
		if m.statusMessage != "" {
			lines = append(lines[:len(lines)-1], m.statusMessage, lines[len(lines)-1])
		}
		return strings.Join(lines, "\n")
	}
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
	lines = append(lines, "keys: "+m.runWatchKeyLegend())
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) renderMenuBody() string {
	if len(m.launchOptions) == 0 {
		return "No launcher actions are configured."
	}
	lines := []string{"Select mode:"}
	for i, option := range m.launchOptions {
		cursor := "  "
		if i == m.selectedOption {
			cursor = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s", cursor, option.Label))
		if option.Description != "" {
			lines = append(lines, "    "+option.Description)
		}
	}
	if m.launching {
		lines = append(lines, "", "Launching selected mode...")
	}
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
	cmds := []tea.Cmd{m.reloadTick()}
	if m.doneCh != nil {
		cmds = append(cmds, m.doneTickCmd())
	}
	return tea.Batch(cmds...)
}

func (m RunWatchModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		snapshot, err := loadRunWatchSnapshot(m.store, m.focusTicketID)
		return runWatchLoadedMsg{snapshot: snapshot, err: err}
	}
}

func (m RunWatchModel) doneTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return runWatchDoneMsg{}
	})
}

func (m RunWatchModel) startSelectedOption() (tea.Model, tea.Cmd) {
	if len(m.launchOptions) == 0 || m.selectedOption >= len(m.launchOptions) {
		return m, nil
	}
	option := m.launchOptions[m.selectedOption]
	if option.Start == nil {
		m.launchMode = launchModeWatch
		m.statusMessage = "watching managed run"
		return m, nil
	}
	m.launching = true
	m.launchMode = launchModeWatch
	m.showDoneNotice = false
	doneCh := make(chan struct{})
	m.doneCh = doneCh
	return m, func() tea.Msg {
		defer close(doneCh)
		return runWatchLaunchResultMsg{err: option.Start()}
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
	return slices.Clone([]string{"j", "k", "enter", "l", "tab", "s", "r", "m", "q"})
}

func (m RunWatchModel) runWatchKeyLegend() string {
	parts := []string{"l/tab toggle logs", "s stop after current", "r refresh"}
	if len(m.launchOptions) > 0 {
		parts = append(parts, "m menu")
	}
	parts = append(parts, "q quit")
	return strings.Join(parts, " | ")
}

func menuKeyLegend() string {
	return "j/k move | enter launch | q quit"
}
