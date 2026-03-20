package tui

import (
	"path/filepath"
	"slices"
	"sort"
	"strconv"
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

var (
	runWatchShellStyle = lipgloss.NewStyle().
				Padding(1, 2)
	runWatchHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("24")).
				Padding(0, 1)
	runWatchSubtleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))
	runWatchCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(1, 2)
	runWatchActiveCardStyle = runWatchCardStyle.Copy().
				BorderForeground(lipgloss.Color("36"))
	runWatchMutedCardStyle = runWatchCardStyle.Copy().
				BorderForeground(lipgloss.Color("240"))
	runWatchKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("229"))
	runWatchHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)
	runWatchStatusOKStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("42"))
	runWatchStatusWarnStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("214"))
	runWatchStatusErrStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("203"))
	runWatchSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("31")).
				Padding(0, 1)
)

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
	return tea.Batch(m.loadCmd(), m.reloadTick())
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
			return m, nil
		}
		m.snapshot = msg.snapshot
		if m.snapshot.ticketID == "" {
			m.statusMessage = "waiting for managed run"
		} else if m.snapshot.cycle.StopAfterCurrent {
			m.statusMessage = "stop requested after current ticket"
		} else {
			m.statusMessage = "watching managed run"
		}
		return m, nil
	case runWatchTickMsg:
		if m.doneCh != nil {
			select {
			case <-m.doneCh:
				m.launching = false
				m.showDoneNotice = true
				if m.quitOnDone {
					return m, tea.Quit
				}
				return m, nil
			default:
			}
		}
		return m, tea.Batch(m.loadCmd(), m.reloadTick())
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
	if m.launchMode == launchModeMenu {
		return m.renderMenuView()
	}
	return m.renderWatchView()
}

func (m RunWatchModel) renderMenuBody() string {
	if len(m.launchOptions) == 0 {
		return "No launcher actions are configured."
	}
	lines := []string{"Select mode"}
	for i, option := range m.launchOptions {
		label := option.Label
		if i == m.selectedOption {
			label = runWatchSelectedStyle.Render("› " + label)
		} else {
			label = "  " + label
		}
		lines = append(lines, label)
		if option.Description != "" {
			lines = append(lines, runWatchSubtleStyle.Render("    "+option.Description))
		}
	}
	if m.launching {
		lines = append(lines, "", runWatchStatusWarnStyle.Render("Launching selected mode..."))
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

func (m RunWatchModel) renderMenuView() string {
	header := m.renderHeader("Managed Run Launcher", "choose a mode and start from the dashboard")
	menu := runWatchActiveCardStyle.Render(m.renderMenuBody())
	help := runWatchHelpStyle.Render("keys: " + menuKeyLegend())
	sections := []string{header, menu}
	if m.statusMessage != "" {
		sections = append(sections, m.renderStatusBanner())
	}
	sections = append(sections, help)
	return runWatchShellStyle.Render(strings.Join(sections, "\n\n"))
}

func (m RunWatchModel) renderWatchView() string {
	header := m.renderHeader("Managed Run Watch", m.renderHeaderMeta())
	mainCardStyle := runWatchActiveCardStyle
	if m.snapshot.ticketID == "" {
		mainCardStyle = runWatchMutedCardStyle
	}
	summaryCard := mainCardStyle.Render(m.renderWatchSummaryCard())
	bodyTitle := "Summary"
	bodyContent := m.renderSummaryBody()
	if m.mode == watchModeLog {
		bodyTitle = "Transcript"
		bodyContent = m.renderLogBody()
	}
	bodyCard := runWatchCardStyle.Render(bodyTitle + "\n\n" + bodyContent)
	sections := []string{header, summaryCard, bodyCard}
	if m.showDoneNotice || m.statusMessage != "" {
		sections = append(sections, m.renderStatusBanner())
	}
	sections = append(sections, runWatchHelpStyle.Render("keys: "+m.runWatchKeyLegend()))
	return runWatchShellStyle.Render(strings.Join(sections, "\n\n"))
}

func (m RunWatchModel) renderHeader(title, subtitle string) string {
	titleLine := runWatchHeaderStyle.Render(title)
	repoLine := runWatchSubtleStyle.Render("repo  " + filepath.Base(m.repoRoot))
	if subtitle == "" {
		return lipgloss.JoinVertical(lipgloss.Left, titleLine, repoLine)
	}
	return lipgloss.JoinVertical(lipgloss.Left, titleLine, repoLine, runWatchSubtleStyle.Render(subtitle))
}

func (m RunWatchModel) renderHeaderMeta() string {
	parts := []string{
		"ticket " + valueOrFallback(m.snapshot.ticketID, "(none)"),
		"mode " + string(m.mode),
	}
	if m.snapshot.cycle.StopAfterCurrent {
		parts = append(parts, "stop after current")
	} else {
		parts = append(parts, "continuous")
	}
	return strings.Join(parts, "  •  ")
}

func (m RunWatchModel) renderWatchSummaryCard() string {
	rows := []string{
		m.renderKeyValue("Ticket", valueOrFallback(m.snapshot.ticketID, "(none)")),
		m.renderKeyValue("Run state", m.renderRunState()),
		m.renderKeyValue("Step", m.renderStepProgress()),
		m.renderKeyValue("Phase", valueOrFallback(m.snapshot.status.CurrentPhase, "waiting")),
		m.renderKeyValue("Last event", valueOrFallback(m.snapshot.status.LastEventAt, "none yet")),
	}
	return "Run Overview\n\n" + strings.Join(rows, "\n")
}

func (m RunWatchModel) renderRunState() string {
	if !m.snapshot.statusOK {
		if m.snapshot.ticketID == "" {
			return runWatchSubtleStyle.Render("idle")
		}
		return runWatchStatusWarnStyle.Render("awaiting status")
	}
	switch {
	case m.snapshot.status.Hung:
		return runWatchStatusErrStyle.Render("hung")
	case m.snapshot.status.Active:
		return runWatchStatusOKStyle.Render("active")
	default:
		return runWatchSubtleStyle.Render("inactive")
	}
}

func (m RunWatchModel) renderStepProgress() string {
	if !m.snapshot.statusOK || m.snapshot.status.CurrentStepTitle == "" {
		return runWatchSubtleStyle.Render("waiting")
	}
	prefix := strconv.Itoa(m.snapshot.status.CurrentStep)
	if m.snapshot.status.PlannedSteps > 0 {
		prefix += "/" + strconv.Itoa(m.snapshot.status.PlannedSteps)
	}
	return prefix + "  " + m.snapshot.status.CurrentStepTitle
}

func (m RunWatchModel) renderStatusBanner() string {
	lines := make([]string, 0, 2)
	if m.showDoneNotice {
		lines = append(lines, runWatchStatusOKStyle.Render("Run finished"))
	}
	if m.statusMessage != "" {
		lines = append(lines, m.statusMessage)
	}
	style := runWatchMutedCardStyle
	if m.snapshot.status.Hung {
		style = runWatchCardStyle.Copy().BorderForeground(lipgloss.Color("203"))
	} else if m.snapshot.cycle.StopAfterCurrent || m.launching {
		style = runWatchCardStyle.Copy().BorderForeground(lipgloss.Color("214"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m RunWatchModel) renderKeyValue(key, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Left,
		runWatchSubtleStyle.Width(12).Render(strings.ToUpper(key)),
		value,
	)
}

func valueOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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
	return renderLegend(
		"j/k", "move",
		"enter", "launch",
		"q", "quit",
	)
}

func renderLegend(parts ...string) string {
	items := make([]string, 0, len(parts)/2)
	for i := 0; i+1 < len(parts); i += 2 {
		items = append(items, runWatchKeyStyle.Render(parts[i])+" "+parts[i+1])
	}
	return strings.Join(items, "  •  ")
}
