package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	charmansi "github.com/charmbracelet/x/ansi"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/store/local"
	"github.com/leomorpho/docket/internal/ticket"
	workablepkg "github.com/leomorpho/docket/internal/workable"
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
	RepoRoot    string
	Start       func() (string, error)
	StayInMenu  bool
}

type runWatchSnapshot struct {
	cycle        runruntime.CycleState
	cycleOK      bool
	ticketID     string
	status       runruntime.StatusSnapshot
	statusOK     bool
	queue        []runWatchQueueItem
	transcript   []runruntime.TranscriptEntry
	conversation []string
	warnings     []string
}

type runWatchQueueItem struct {
	TicketID string
	Title    string
}

type runWatchLoadedMsg struct {
	snapshot runWatchSnapshot
	err      error
}

type runWatchDoneMsg struct{}
type runWatchTickMsg struct{}
type runWatchLaunchResultMsg struct {
	err             error
	terminalMessage string
	stayInMenu      bool
}

type RunWatchModel struct {
	repoRoot        string
	store           *runruntime.Store
	focusTicketID   string
	launchMode      launchMode
	launchOptions   []RunWatchLaunchOption
	selectedOption  int
	launching       bool
	mode            watchMode
	showOverview    bool
	followLog       bool
	scrollOffset    int
	width           int
	height          int
	statusMessage   string
	snapshot        runWatchSnapshot
	doneCh          <-chan struct{}
	quitOnDone      bool
	showDoneNotice  bool
	showHelp        bool
	confirmHardStop bool
	terminalMessage string
}

var (
	runWatchShellStyle = lipgloss.NewStyle().
				Padding(0, 2)
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
	runWatchCompactCardStyle = runWatchCardStyle.Copy().
					Padding(0, 1)
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
	runWatchProgressDoneStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("42"))
	runWatchProgressTodoStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("240"))
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
		mode:          watchModeLog,
		showOverview:  true,
		followLog:     true,
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
		prevTranscriptLen := len(m.snapshot.transcript)
		if msg.err != nil {
			m.statusMessage = "watch refresh failed: " + msg.err.Error()
			return m, nil
		}
		m.snapshot = msg.snapshot
		if len(msg.snapshot.warnings) > 0 {
			m.statusMessage = strings.Join(msg.snapshot.warnings, " | ")
		}
		if m.followLog && len(m.snapshot.transcript) >= prevTranscriptLen {
			m.scrollOffset = max(0, len(m.snapshot.transcript)-m.bodyViewportHeight())
		}
		if len(msg.snapshot.warnings) == 0 && m.snapshot.ticketID == "" {
			if strings.TrimSpace(m.terminalMessage) != "" {
				m.statusMessage = m.terminalMessage
			} else {
				m.statusMessage = "waiting for managed run"
			}
		} else if len(msg.snapshot.warnings) == 0 && m.snapshot.cycle.StopAfterCurrent {
			m.statusMessage = "stop requested after current ticket"
		} else if len(msg.snapshot.warnings) == 0 {
			m.terminalMessage = ""
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
			m.terminalMessage = ""
			m.launchMode = launchModeMenu
			return m, nil
		}
		m.showDoneNotice = true
		m.terminalMessage = strings.TrimSpace(msg.terminalMessage)
		if msg.stayInMenu {
			m.launchMode = launchModeMenu
			m.statusMessage = "action completed"
		} else {
			m.launchMode = launchModeWatch
			if m.terminalMessage != "" {
				m.statusMessage = m.terminalMessage
			} else {
				m.statusMessage = "run finished"
			}
		}
		return m, m.loadCmd()
	case tea.MouseMsg:
		if m.launchMode != launchModeWatch {
			return m, nil
		}
		switch {
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp:
			if m.scrollOffset > 0 {
				m.followLog = false
				m.scrollOffset--
			}
			return m, nil
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown:
			m.followLog = false
			m.scrollOffset++
			return m, nil
		}
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
			} else if m.launchMode == launchModeWatch && m.scrollOffset > 0 {
				m.followLog = false
				m.scrollOffset--
			}
			return m, nil
		case "down", "j":
			if m.launchMode == launchModeMenu && len(m.launchOptions) > 0 {
				m.selectedOption = (m.selectedOption + 1) % len(m.launchOptions)
			} else if m.launchMode == launchModeWatch {
				m.followLog = false
				m.scrollOffset++
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
			m.scrollOffset = 0
			m.followLog = true
			return m, nil
		case "?":
			if m.launchMode == launchModeWatch {
				m.showHelp = !m.showHelp
			}
			return m, nil
		case "g":
			if m.launchMode == launchModeWatch {
				m.followLog = false
				m.scrollOffset = 0
			}
			return m, nil
		case "G":
			if m.launchMode == launchModeWatch {
				m.followLog = true
				m.scrollOffset = max(0, len(m.snapshot.transcript)-m.bodyViewportHeight())
			}
			return m, nil
		case "h":
			if m.launchMode == launchModeWatch {
				m.showOverview = !m.showOverview
			}
			return m, nil
		case "s":
			if m.launchMode != launchModeWatch {
				return m, nil
			}
			m.confirmHardStop = false
			if err := m.store.RequestStopAfterCurrent(time.Now()); err != nil {
				m.statusMessage = "failed to request stop: " + err.Error()
				return m, nil
			}
			m.statusMessage = "stop requested after current ticket"
			m.snapshot.cycle.StopAfterCurrent = true
			return m, nil
		case "x":
			if m.launchMode != launchModeWatch || m.snapshot.ticketID == "" || !m.snapshot.status.Active {
				return m, nil
			}
			if !m.confirmHardStop {
				m.confirmHardStop = true
				m.statusMessage = "press x again to hard stop the current run"
				return m, nil
			}
			m.confirmHardStop = false
			if err := m.store.HardStopRun(m.snapshot.ticketID, time.Now()); err != nil {
				m.statusMessage = "failed to hard stop run: " + err.Error()
				return m, nil
			}
			m.statusMessage = "hard stop requested for current run"
			m.snapshot.status.Active = false
			m.snapshot.status.Hung = false
			m.snapshot.status.LastResultStatus = "stopped"
			m.snapshot.status.LastVisibleText = "Operator requested hard stop"
			return m, nil
		case "p":
			if m.launchMode != launchModeWatch {
				return m, nil
			}
			return m.startLaunchOptionByID("ping")
		case "r":
			return m, m.loadCmd()
		case "m", "esc":
			if len(m.launchOptions) > 0 && !m.launching {
				m.launchMode = launchModeMenu
				m.statusMessage = "choose a managed run mode"
				m.confirmHardStop = false
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
		return "No active managed run detected.\n\nPress m to open the launcher menu and start or attach."
	}
	body := []string{}
	if m.snapshot.status.LastVisibleText != "" {
		body = append(body, "Last visible:")
		body = append(body, "  "+m.snapshot.status.LastVisibleText)
	}
	if m.snapshot.status.LastEventAt != "" {
		body = append(body, "Last event: "+formatRuntimeTimestampWithRelative(m.snapshot.status.LastEventAt))
	}
	if m.snapshot.status.LastHealthCheck != "" {
		body = append(body, "Last health check:")
		body = append(body, "  "+m.snapshot.status.LastHealthCheck)
	}
	if m.snapshot.status.LastIntervention != "" {
		body = append(body, "Last intervention: "+m.snapshot.status.LastIntervention)
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
	if m.snapshot.ticketID == "" {
		return "No active managed run detected.\n\nOpen the launcher menu with m to start the next ticket, start an auto cycle, or clean stale runs."
	}
	if len(m.snapshot.transcript) == 0 {
		return "No visible transcript yet.\n\nThe run has started, but no user-visible messages have been captured so far."
	}
	lines := make([]string, 0, len(m.snapshot.transcript))
	for _, entry := range m.snapshot.transcript {
		lines = append(lines, entry.Text)
	}
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) renderConversationBody() string {
	if m.snapshot.ticketID == "" {
		return "No active managed run detected.\n\nThe raw Codex session view will appear once a managed run starts."
	}
	if len(m.snapshot.conversation) == 0 {
		return "No raw Codex conversation captured yet.\n\nThis pane populates from the persisted stdout event stream."
	}
	return strings.Join(m.snapshot.conversation, "\n")
}

func (m RunWatchModel) renderMenuView() string {
	header := m.renderHeader("Managed Run Launcher", "choose a mode and start from the dashboard")
	contentWidth := m.contentWidth()
	menu := runWatchActiveCardStyle.Copy().Width(contentWidth).Render(m.renderMenuBody())
	help := runWatchHelpStyle.Render("keys: " + menuKeyLegend())
	sections := []string{header, menu}
	if m.shouldRenderStatusBanner() {
		sections = append(sections, m.renderStatusBanner(contentWidth))
	}
	sections = append(sections, help)
	return runWatchShellStyle.Render(m.padToHeight(strings.Join(sections, "\n\n")))
}

func (m RunWatchModel) renderWatchView() string {
	header := m.renderHeader("Managed Run Watch", m.renderHeaderMeta())
	contentWidth := m.contentWidth()
	sections := []string{header}
	statusBanner := ""
	if m.shouldRenderStatusBanner() {
		statusBanner = m.renderStatusBanner(contentWidth)
	}
	footer := m.renderFooter(contentWidth)
	summaryOuterHeight, summaryInnerHeight, bodyOuterHeight, bodyInnerHeight := m.watchLayoutHeights(header, statusBanner, footer)
	if m.showOverview {
		mainCardStyle := runWatchActiveCardStyle.Copy().Width(contentWidth)
		if m.snapshot.ticketID == "" {
			mainCardStyle = runWatchMutedCardStyle.Copy().Width(contentWidth)
		}
		summaryCard := runWatchCompactCardStyle.Copy().Inherit(mainCardStyle).Width(contentWidth).Height(summaryOuterHeight).Render(m.renderWatchSummaryCard(contentWidth, summaryInnerHeight))
		sections = append(sections, summaryCard)
	}
	bodyTitle := "Visible Session Log"
	bodyContent := m.renderLogBody()
	if m.mode == watchModeLog {
		leftWidth := max(30, (contentWidth-2)/2)
		rightWidth := max(30, contentWidth-leftWidth-2)
		leftCard := runWatchCompactCardStyle.Copy().Width(leftWidth).Height(bodyOuterHeight).Render("Visible Session Log\n\n" + m.renderScrollableBody(m.renderLogBody(), leftWidth, bodyInnerHeight))
		rightCard := runWatchCompactCardStyle.Copy().Width(rightWidth).Height(bodyOuterHeight).Render("Codex Session Transcript\n\n" + m.renderScrollableBody(m.renderConversationBody(), rightWidth, bodyInnerHeight))
		bodyTitle = ""
		bodyContent = lipgloss.JoinHorizontal(lipgloss.Top, leftCard, "  ", rightCard)
	} else {
		bodyTitle = "Run Summary"
		bodyContent = m.renderSummaryBody()
	}
	bodyCard := bodyContent
	if m.mode != watchModeLog {
		bodyCard = runWatchCompactCardStyle.Copy().Width(contentWidth).Height(bodyOuterHeight).Render(bodyTitle + "\n\n" + m.renderScrollableBody(bodyContent, contentWidth, bodyInnerHeight))
	}

	sections = append(sections, bodyCard)
	if statusBanner != "" {
		sections = append(sections, statusBanner)
	}
	sections = append(sections, footer)
	content := strings.Join(sections, "\n")
	return terminalTitle(m.terminalTitle()) + runWatchShellStyle.Render(m.padToHeight(content))
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
	if completed := len(m.snapshot.cycle.Completed); completed > 0 {
		parts = append(parts, fmt.Sprintf("done %d", completed))
	}
	if m.snapshot.cycle.StopAfterCurrent {
		parts = append(parts, "stop after current")
	} else {
		parts = append(parts, "continuous")
	}
	return strings.Join(parts, "  •  ")
}

func (m RunWatchModel) renderWatchSummaryCard(contentWidth int, viewportHeight int) string {
	leftWidth := max(28, (contentWidth-10)/2)
	rightWidth := max(28, contentWidth-leftWidth-6)

	leftBody := m.renderFixedBody("Ticket Stats", m.renderTicketStatsBody(), leftWidth, viewportHeight)
	rightBody := m.renderFixedBody("General Stats", m.renderGeneralOverviewBody(), rightWidth, viewportHeight)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftBody, "  ", rightBody)
}

func (m RunWatchModel) renderTicketStatsBody() string {
	progress := strings.TrimSpace(stripLipglossANSI(m.renderStepBar()))
	if progress == "" {
		progress = "waiting"
	}
	phase := strings.TrimSpace(valueOrFallback(m.snapshot.status.CurrentPhase, "waiting"))
	lines := []string{
		fmt.Sprintf("%s  %s", valueOrFallback(m.snapshot.ticketID, "(none)"), stripLipglossANSI(m.renderRunState())),
		fmt.Sprintf("Step: %s", stripLipglossANSI(m.renderStepProgress())),
		fmt.Sprintf("Progress: %s  •  Phase: %s", progress, phase),
		fmt.Sprintf("Last: %s", formattedRuntimeTimestampWithRelativeOrFallback(m.snapshot.status.LastEventAt, "none yet")),
	}
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) renderRunState() string {
	switch m.rawRunState() {
	case "hung":
		return runWatchStatusErrStyle.Render("hung")
	case "active":
		return runWatchStatusOKStyle.Render("active")
	case "stopped":
		return runWatchStatusWarnStyle.Render("stopped")
	case "finished":
		return runWatchStatusOKStyle.Render("finished")
	case "failed":
		return runWatchStatusErrStyle.Render("failed")
	default:
		return runWatchSubtleStyle.Render("inactive")
	}
}

func (m RunWatchModel) rawRunState() string {
	if !m.snapshot.statusOK {
		if m.snapshot.ticketID == "" {
			return "idle"
		}
		return "awaiting status"
	}
	switch {
	case m.snapshot.status.Hung:
		return "hung"
	case m.snapshot.status.Active:
		return "active"
	case m.snapshot.status.LastResultStatus == "stopped":
		return "stopped"
	case m.snapshot.status.LastResultStatus == string("done"):
		return "finished"
	case m.snapshot.status.LastResultStatus == string("failed"):
		return "failed"
	default:
		return "inactive"
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

func (m RunWatchModel) renderStepBar() string {
	if !m.snapshot.statusOK || m.snapshot.status.PlannedSteps <= 0 {
		return runWatchSubtleStyle.Render("waiting")
	}
	current := m.snapshot.status.CurrentStep
	if current < 0 {
		current = 0
	}
	if current > m.snapshot.status.PlannedSteps {
		current = m.snapshot.status.PlannedSteps
	}
	const width = 12
	filled := int(float64(current) / float64(m.snapshot.status.PlannedSteps) * float64(width))
	if current > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	bar := runWatchProgressDoneStyle.Render(strings.Repeat("█", filled)) +
		runWatchProgressTodoStyle.Render(strings.Repeat("░", width-filled))
	percent := int(float64(current) / float64(m.snapshot.status.PlannedSteps) * 100)
	return fmt.Sprintf("%s %d%%", bar, percent)
}

func (m RunWatchModel) renderStatusBanner(contentWidth int) string {
	lines := make([]string, 0, 2)
	if m.showDoneNotice {
		lines = append(lines, runWatchStatusOKStyle.Render("Run finished"))
	}
	if m.shouldRenderStatusBanner() {
		lines = append(lines, m.statusMessage)
	}
	style := runWatchMutedCardStyle
	if m.snapshot.status.Hung {
		style = runWatchCardStyle.Copy().BorderForeground(lipgloss.Color("203"))
	} else if m.snapshot.cycle.StopAfterCurrent || m.launching {
		style = runWatchCardStyle.Copy().BorderForeground(lipgloss.Color("214"))
	}
	style = style.Width(contentWidth)
	return style.Render(strings.Join(lines, "\n"))
}

func (m RunWatchModel) renderKeyValue(key, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Left,
		runWatchSubtleStyle.Width(14).Render(strings.ToUpper(key)),
		"  ",
		value,
	)
}

func (m RunWatchModel) renderScrollableBody(content string, contentWidth int, viewportHeight int) string {
	innerWidth := max(20, contentWidth-6)
	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(content)
	lines := strings.Split(wrapped, "\n")
	if viewportHeight <= 0 || len(lines) <= viewportHeight {
		marker := "showing all lines"
		if m.followLog {
			marker += "  •  follow"
		}
		hint := runWatchSubtleStyle.Render(marker)
		return strings.TrimRight(wrapped, "\n") + "\n\n" + hint
	}
	maxOffset := len(lines) - viewportHeight
	offset := m.scrollOffset
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	maxOffset = max(0, maxOffset)
	if offset > maxOffset {
		offset = maxOffset
	}
	visible := lines[offset : offset+viewportHeight]
	position := "scroll " + strconv.Itoa(offset+1) + "-" + strconv.Itoa(offset+len(visible)) + " of " + strconv.Itoa(len(lines))
	if m.followLog {
		position += "  •  follow"
	}
	topMarker := runWatchSubtleStyle.Render(position)
	return strings.Join(visible, "\n") + "\n\n" + topMarker
}

func (m RunWatchModel) renderFixedBody(title, content string, contentWidth int, viewportHeight int) string {
	innerWidth := max(20, contentWidth-2)
	bodyHeight := max(3, viewportHeight-1)
	body := m.clampBodyLines(content, innerWidth, bodyHeight)
	return title + "\n" + body
}

func (m RunWatchModel) clampBodyLines(content string, contentWidth int, viewportHeight int) string {
	wrapped := lipgloss.NewStyle().Width(contentWidth).Render(content)
	lines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	if viewportHeight <= 0 || len(lines) <= viewportHeight {
		return strings.Join(lines, "\n")
	}
	if viewportHeight == 1 {
		return runWatchSubtleStyle.Render("…")
	}
	hidden := len(lines) - (viewportHeight - 1)
	visible := append([]string{}, lines[:viewportHeight-1]...)
	visible = append(visible, runWatchSubtleStyle.Render(fmt.Sprintf("… +%d more", hidden)))
	return strings.Join(visible, "\n")
}

func (m RunWatchModel) renderGeneralOverviewBody() string {
	lines := []string{fmt.Sprintf("Planned Queue (%d): %s", len(m.snapshot.queue), m.renderQueueSummary())}
	if len(m.snapshot.cycle.Completed) == 0 {
		lines = append(lines, "Done This Session (0): none")
	} else {
		lines = append(lines, fmt.Sprintf("Done This Session (%d): %s", len(m.snapshot.cycle.Completed), m.renderCompletedSummary()))
	}
	lines = append(lines, "Cycle: "+stripLipglossANSI(m.renderCycleMode()))
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) renderQueueSummary() string {
	if len(m.snapshot.queue) == 0 {
		return "none"
	}
	items := make([]string, 0, minInt(3, len(m.snapshot.queue)))
	for _, item := range m.snapshot.queue[:minInt(3, len(m.snapshot.queue))] {
		items = append(items, item.TicketID)
	}
	if len(m.snapshot.queue) > len(items) {
		items = append(items, fmt.Sprintf("+%d more", len(m.snapshot.queue)-len(items)))
	}
	return strings.Join(items, ", ")
}

func (m RunWatchModel) renderCompletedSummary() string {
	items := make([]string, 0, minInt(3, len(m.snapshot.cycle.Completed)))
	for _, item := range m.snapshot.cycle.Completed[:minInt(3, len(m.snapshot.cycle.Completed))] {
		label := item.TicketID
		if strings.TrimSpace(item.Status) != "" {
			label += " [" + item.Status + "]"
		}
		if strings.TrimSpace(item.Length) != "" {
			label += " " + item.Length
		}
		items = append(items, label)
	}
	if len(m.snapshot.cycle.Completed) > len(items) {
		items = append(items, fmt.Sprintf("+%d more", len(m.snapshot.cycle.Completed)-len(items)))
	}
	return strings.Join(items, ", ")
}

func (m RunWatchModel) renderCycleMode() string {
	if m.snapshot.cycle.StopAfterCurrent {
		return runWatchStatusWarnStyle.Render("stop after current")
	}
	if m.snapshot.cycle.Active || m.snapshot.ticketID != "" {
		return runWatchStatusOKStyle.Render("continuous")
	}
	return runWatchSubtleStyle.Render("idle")
}

func (m RunWatchModel) renderFooter(contentWidth int) string {
	help := runWatchHelpStyle.Render("keys: " + m.runWatchKeyLegend())
	status := runWatchSubtleStyle.Render("")
	if m.showDoneNotice {
		status = runWatchStatusOKStyle.Render("run finished")
	} else if m.shouldRenderFooterStatus() {
		status = runWatchSubtleStyle.Render(m.statusMessage)
	}
	if status == "" {
		status = runWatchSubtleStyle.Render(" ")
	}
	if m.confirmHardStop {
		status = runWatchStatusErrStyle.Render("hard stop armed: press x again to confirm")
	}
	footer := ""
	if contentWidth < lipgloss.Width(help)+lipgloss.Width(status)+2 {
		footer = help + "\n" + lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Right).Render(status)
	} else {
		footer = lipgloss.JoinHorizontal(
			lipgloss.Top,
			help,
			lipgloss.NewStyle().Width(max(0, contentWidth-lipgloss.Width(help))).Align(lipgloss.Right).Render(status),
		)
	}
	if m.showHelp {
		helpCard := runWatchMutedCardStyle.Copy().Width(contentWidth).Render(m.renderHelpBody())
		return helpCard + "\n\n" + footer
	}
	return footer
}

func (m RunWatchModel) shouldRenderStatusBanner() bool {
	switch strings.TrimSpace(m.statusMessage) {
	case "", "watching managed run", "waiting for managed run":
		return false
	default:
		return true
	}
}

func (m RunWatchModel) shouldRenderFooterStatus() bool {
	switch strings.TrimSpace(m.statusMessage) {
	case "", "watching managed run":
		return false
	default:
		return true
	}
}

func (m RunWatchModel) padToHeight(content string) string {
	if m.height <= 0 {
		return content
	}
	contentLines := lipgloss.Height(content)
	target := max(0, m.height-2)
	if contentLines >= target {
		return content
	}
	return content + strings.Repeat("\n", target-contentLines)
}

func (m RunWatchModel) contentWidth() int {
	if m.width <= 0 {
		return 96
	}
	return max(40, m.width-6)
}

func (m RunWatchModel) bodyViewportHeight() int {
	if m.height <= 0 {
		return 14
	}
	base := m.height - 18
	if base < 6 {
		return 6
	}
	return base
}

func (m RunWatchModel) watchLayoutHeights(header, statusBanner, footer string) (int, int, int, int) {
	bodyOuter := m.bodyViewportHeight()
	summaryOuter := 0
	if m.height > 0 {
		target := max(12, m.height-2)
		fixedHeight := 0
		visibleSections := 0
		for _, section := range []string{header, statusBanner, footer} {
			if strings.TrimSpace(section) == "" {
				continue
			}
			fixedHeight += lipgloss.Height(section)
			visibleSections++
		}
		totalSections := visibleSections + 1
		if m.showOverview {
			totalSections++
		}
		separatorLines := max(0, totalSections-1)
		available := max(8, target-fixedHeight-separatorLines)
		if m.showOverview {
			summaryOuter = minInt(11, max(7, available/3))
			bodyOuter = max(6, available-summaryOuter)
			if bodyOuter < 8 && summaryOuter > 7 {
				needed := minInt(summaryOuter-7, 8-bodyOuter)
				summaryOuter -= needed
				bodyOuter += needed
			}
		} else {
			bodyOuter = available
		}
	}
	if m.showOverview && summaryOuter == 0 {
		summaryOuter = 7
	}
	summaryInner := max(5, summaryOuter-2)
	bodyInner := max(3, bodyOuter-4)
	return summaryOuter, summaryInner, bodyOuter, bodyInner
}

func valueOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formattedRuntimeTimestampOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return formatRuntimeTimestamp(value)
}

func formattedRuntimeTimestampWithRelativeOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return formatRuntimeTimestampWithRelative(value)
}

func formatRuntimeDurationOrFallback(startedAt, fallback string) string {
	startedAt = strings.TrimSpace(startedAt)
	if startedAt == "" {
		return fallback
	}
	parsed, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return fallback
	}
	d := time.Since(parsed)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "under 1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
}

func formatRuntimeTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.In(time.Local).Format("Jan 2, 2006 3:04:05 PM MST")
}

func formatRuntimeTimestampWithRelative(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	local := parsed.In(time.Local)
	return local.Format("Jan 2, 2006 3:04:05 PM MST") + " (" + relativeTime(local, time.Now().In(time.Local)) + ")"
}

func relativeTime(then, now time.Time) string {
	if now.Before(then) {
		then, now = now, then
	}
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (m RunWatchModel) renderHelpBody() string {
	lines := []string{
		"Help",
		"",
		"l/tab  toggle summary and visible log",
		"j/k    scroll log",
		"g/G    jump top / jump bottom and follow",
		"h      hide or show overview",
		"s      stop after current ticket",
		"x      hard stop current run (press twice to confirm)",
		"r      refresh",
		"?      toggle help",
	}
	if len(m.launchOptions) > 0 {
		lines = append(lines, "m      open launcher menu")
	}
	lines = append(lines, "q      quit dashboard")
	return strings.Join(lines, "\n")
}

func (m RunWatchModel) terminalTitle() string {
	parts := []string{filepath.Base(m.repoRoot)}
	if m.snapshot.ticketID != "" {
		parts = append(parts, m.snapshot.ticketID)
	}
	if m.snapshot.status.CurrentPhase != "" {
		parts = append(parts, m.snapshot.status.CurrentPhase)
	} else {
		parts = append(parts, m.rawRunState())
	}
	if m.snapshot.status.PlannedSteps > 0 && m.snapshot.status.CurrentStep > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.snapshot.status.CurrentStep, m.snapshot.status.PlannedSteps))
	}
	return strings.Join(parts, " • ")
}

func terminalTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return "\x1b]0;" + title + "\x07"
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
		snapshot, err := loadRunWatchSnapshot(m.store, m.repoRoot, m.focusTicketID)
		return runWatchLoadedMsg{snapshot: snapshot, err: err}
	}
}

func (m RunWatchModel) startSelectedOption() (tea.Model, tea.Cmd) {
	if len(m.launchOptions) == 0 || m.selectedOption >= len(m.launchOptions) {
		return m, nil
	}
	option := m.launchOptions[m.selectedOption]
	return m.startLaunchOption(option)
}

func (m RunWatchModel) startLaunchOptionByID(id string) (tea.Model, tea.Cmd) {
	for _, option := range m.launchOptions {
		if option.ID == id {
			return m.startLaunchOption(option)
		}
	}
	if strings.TrimSpace(id) == "ping" {
		m.statusMessage = "ping is unavailable in this watch mode"
	}
	return m, nil
}

func (m RunWatchModel) startLaunchOption(option RunWatchLaunchOption) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(option.RepoRoot) != "" && option.RepoRoot != m.repoRoot {
		m.repoRoot = option.RepoRoot
		m.store = runruntime.New(option.RepoRoot)
		m.focusTicketID = ""
		m.snapshot = runWatchSnapshot{}
	}
	if option.Start != nil && !option.StayInMenu && m.snapshot.status.Active && strings.TrimSpace(m.snapshot.ticketID) != "" {
		m.launchMode = launchModeWatch
		m.showDoneNotice = false
		m.terminalMessage = ""
		m.statusMessage = "watching active managed run"
		return m, nil
	}
	if option.Start == nil {
		m.launchMode = launchModeWatch
		m.statusMessage = "watching managed run"
		return m, nil
	}
	m.launching = true
	if !option.StayInMenu {
		m.launchMode = launchModeWatch
	}
	m.showDoneNotice = false
	doneCh := make(chan struct{})
	m.doneCh = doneCh
	return m, func() tea.Msg {
		defer close(doneCh)
		message, err := option.Start()
		return runWatchLaunchResultMsg{
			err:             err,
			terminalMessage: message,
			stayInMenu:      option.StayInMenu,
		}
	}
}

func loadRunWatchSnapshot(store *runruntime.Store, repoRoot string, focusTicketID string) (runWatchSnapshot, error) {
	var snapshot runWatchSnapshot
	if store == nil {
		return snapshot, nil
	}
	if warnings, err := store.HealRuntimeState(time.Now()); err == nil {
		snapshot.warnings = append(snapshot.warnings, warnings...)
	} else {
		snapshot.warnings = append(snapshot.warnings, "runtime health check failed: "+err.Error())
	}
	cycle, ok, err := store.LoadCycleState()
	if err != nil {
		snapshot.warnings = append(snapshot.warnings, "cycle state unavailable: "+err.Error())
	}
	snapshot.cycle = cycle
	snapshot.cycleOK = ok
	ticketID, status, statusOK, warnings := selectWatchedTicket(store, focusTicketID, cycle)
	snapshot.warnings = append(snapshot.warnings, warnings...)
	snapshot.ticketID = ticketID
	snapshot.status = status
	snapshot.statusOK = statusOK
	if statusOK && strings.TrimSpace(status.Warning) != "" {
		snapshot.warnings = append(snapshot.warnings, "run warning: "+strings.TrimSpace(status.Warning))
	}
	if queue, err := loadRunWatchQueue(repoRoot, ticketID, cycle.Completed); err != nil {
		snapshot.warnings = append(snapshot.warnings, "planned queue unavailable: "+err.Error())
	} else {
		snapshot.queue = queue
	}
	if ticketID != "" {
		transcript, err := store.LoadTranscript(ticketID)
		if err != nil {
			snapshot.warnings = append(snapshot.warnings, "transcript unavailable for "+ticketID+": "+err.Error())
		} else {
			snapshot.transcript = transcript
		}
		if sessionLines, ok, err := loadCodexSessionLines(status.SessionID); err != nil {
			snapshot.warnings = append(snapshot.warnings, "codex session unavailable for "+ticketID+": "+err.Error())
		} else if ok {
			snapshot.conversation = parseCodexSessionConversation(sessionLines)
		} else {
			stdoutLines, err := store.LoadStdoutLines(ticketID)
			if err != nil {
				snapshot.warnings = append(snapshot.warnings, "raw stdout unavailable for "+ticketID+": "+err.Error())
			} else {
				snapshot.conversation = parseCodexConversation(stdoutLines)
			}
		}
	}
	return snapshot, nil
}

func loadRunWatchQueue(repoRoot, currentTicketID string, completed []runruntime.CycleCompletedRun) ([]runWatchQueueItem, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".docket")); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		return nil, nil
	}
	s := local.New(repoRoot)
	if err := s.SyncIndex(context.Background()); err != nil {
		return nil, err
	}
	tickets, err := workablepkg.Tickets(context.Background(), s, cfg, store.Filter{})
	if err != nil {
		return nil, err
	}
	doneSet := make(map[string]struct{}, len(completed)+1)
	if strings.TrimSpace(currentTicketID) != "" {
		doneSet[currentTicketID] = struct{}{}
	}
	for _, item := range completed {
		doneSet[strings.TrimSpace(item.TicketID)] = struct{}{}
	}
	queue := make([]runWatchQueueItem, 0, len(tickets))
	for _, t := range tickets {
		if t == nil {
			continue
		}
		if _, skip := doneSet[strings.TrimSpace(t.ID)]; skip {
			continue
		}
		queue = append(queue, runWatchQueueItem{
			TicketID: t.ID,
			Title:    strings.TrimSpace(t.Title),
		})
	}
	return queue, nil
}

func loadCodexSessionLines(sessionID string) ([]string, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, false, err
	}
	for _, root := range []string{
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, ".codex", "archived_sessions"),
	} {
		path, found, err := findCodexSessionFile(root, sessionID)
		if err != nil {
			return nil, false, err
		}
		if !found {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false, err
		}
		var lines []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lines = append(lines, line)
		}
		return lines, true, nil
	}
	return nil, false, nil
}

func findCodexSessionFile(root, sessionID string) (string, bool, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, nil
	}
	pattern := "*" + sessionID + ".jsonl"
	if matches, err := filepath.Glob(filepath.Join(root, pattern)); err == nil && len(matches) > 0 {
		return matches[0], true, nil
	}
	if matches, err := filepath.Glob(filepath.Join(root, "*", "*", "*", pattern)); err == nil && len(matches) > 0 {
		return matches[0], true, nil
	}
	return "", false, nil
}

func parseCodexConversation(lines []string) []string {
	var out []string
	for _, line := range lines {
		for _, item := range codexConversationLines(line) {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
	}
	return out
}

func parseCodexSessionConversation(lines []string) []string {
	var out []string
	for _, line := range lines {
		for _, item := range codexSessionConversationLines(line) {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
	}
	return out
}

func codexSessionConversationLines(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return []string{"raw: " + line}
	}
	eventType, _ := event["type"].(string)
	payload, _ := event["payload"].(map[string]any)
	switch eventType {
	case "session_meta":
		if id, _ := payload["id"].(string); strings.TrimSpace(id) != "" {
			return []string{"session: started " + id}
		}
		return []string{"session: started"}
	case "turn_context":
		return []string{"session: turn context"}
	case "event_msg":
		return codexSessionEventMsgLines(payload)
	case "response_item":
		return codexSessionResponseItemLines(payload)
	case "token_count":
		return nil
	default:
		if strings.TrimSpace(eventType) != "" {
			return []string{"event: " + eventType}
		}
		return []string{"raw: " + line}
	}
}

func codexSessionEventMsgLines(payload map[string]any) []string {
	msgType, _ := payload["type"].(string)
	switch msgType {
	case "user_message":
		if msg, _ := payload["message"].(string); strings.TrimSpace(msg) != "" {
			return prefixedLines("user", msg)
		}
	case "agent_message":
		if msg, _ := payload["message"].(string); strings.TrimSpace(msg) != "" {
			return prefixedLines("assistant", msg)
		}
	case "task_started":
		return []string{"session: task started"}
	}
	return nil
}

func codexSessionResponseItemLines(payload map[string]any) []string {
	itemType, _ := payload["type"].(string)
	switch itemType {
	case "message":
		role, _ := payload["role"].(string)
		prefix := conversationPrefix(role + "_message")
		if lines := collectConversationTextValues(prefix, payload); len(lines) > 0 {
			return lines
		}
		return []string{prefix + ": [message]"}
	case "function_call":
		name, _ := payload["name"].(string)
		arguments, _ := payload["arguments"].(string)
		if strings.TrimSpace(name) == "" {
			return []string{"tool: call"}
		}
		if strings.TrimSpace(arguments) != "" {
			return prefixedLines("tool", fmt.Sprintf("call %s %s", name, arguments))
		}
		return []string{"tool: call " + name}
	case "function_call_output":
		if output, _ := payload["output"].(string); strings.TrimSpace(output) != "" {
			return prefixedLines("tool", output)
		}
		return []string{"tool: function output"}
	case "custom_tool_call":
		name, _ := payload["name"].(string)
		status, _ := payload["status"].(string)
		input, _ := payload["input"].(string)
		label := strings.TrimSpace(name)
		if label == "" {
			label = "custom tool"
		}
		switch {
		case strings.TrimSpace(status) != "" && strings.TrimSpace(input) != "":
			return prefixedLines("tool", fmt.Sprintf("%s [%s]\n%s", label, status, input))
		case strings.TrimSpace(status) != "":
			return []string{"tool: " + label + " [" + status + "]"}
		case strings.TrimSpace(input) != "":
			return prefixedLines("tool", fmt.Sprintf("%s\n%s", label, input))
		default:
			return []string{"tool: " + label}
		}
	case "reasoning":
		return nil
	default:
		if strings.TrimSpace(itemType) != "" {
			return []string{"item: " + itemType}
		}
		return nil
	}
}

func codexConversationLines(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return []string{"raw: " + line}
	}
	eventType, _ := event["type"].(string)
	switch eventType {
	case "thread.started":
		if threadID, _ := event["thread_id"].(string); strings.TrimSpace(threadID) != "" {
			return []string{"session: thread started " + threadID}
		}
		return []string{"session: thread started"}
	case "turn.started":
		return []string{"session: turn started"}
	case "turn.completed":
		return []string{"session: turn completed"}
	}
	item, _ := event["item"].(map[string]any)
	if len(item) == 0 {
		if strings.TrimSpace(eventType) != "" {
			return []string{"event: " + eventType}
		}
		return []string{"raw: " + line}
	}
	itemType, _ := item["type"].(string)
	prefix := conversationPrefix(itemType)
	switch itemType {
	case "tool_call":
		if lines := toolCallConversationLines(item); len(lines) > 0 {
			return lines
		}
	}
	if lines := collectConversationTextValues(prefix, item); len(lines) > 0 {
		return lines
	}
	if strings.TrimSpace(itemType) != "" {
		return []string{prefix + ": [" + itemType + "]"}
	}
	return []string{"raw: " + line}
}

func conversationPrefix(itemType string) string {
	switch strings.TrimSpace(itemType) {
	case "user_message":
		return "user"
	case "developer_message":
		return "developer"
	case "tool_result", "tool_call", "tool_message":
		return "tool"
	case "error":
		return "error"
	case "system_message":
		return "system"
	default:
		return "assistant"
	}
}

func collectConversationTextValues(prefix string, value any) []string {
	seen := map[string]struct{}{}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch typed := v.(type) {
		case map[string]any:
			for _, key := range []string{"text", "delta", "message"} {
				if str, _ := typed[key].(string); strings.TrimSpace(str) != "" {
					for _, line := range prefixedLines(prefix, str) {
						if _, ok := seen[line]; ok {
							continue
						}
						seen[line] = struct{}{}
						out = append(out, line)
					}
				}
			}
			keys := make([]string, 0, len(typed))
			for key := range typed {
				if key == "text" || key == "delta" || key == "message" {
					continue
				}
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}

func toolCallConversationLines(item map[string]any) []string {
	var out []string
	name, _ := item["name"].(string)
	name = strings.TrimSpace(name)
	arguments, _ := item["arguments"].(string)
	arguments = strings.TrimSpace(arguments)
	switch {
	case name != "" && arguments != "":
		out = append(out, prefixedLines("tool", fmt.Sprintf("call %s %s", name, arguments))...)
	case name != "":
		out = append(out, "tool: call "+name)
	}
	out = append(out, collectConversationTextValues("tool", item)...)
	return out
}

func prefixedLines(prefix, text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, prefix+": "+line)
	}
	return out
}

func stripLipglossANSI(value string) string {
	return strings.TrimSpace(charmansi.Strip(value))
}

func selectWatchedTicket(store *runruntime.Store, focusTicketID string, cycle runruntime.CycleState) (string, runruntime.StatusSnapshot, bool, []string) {
	var warnings []string
	if focusTicketID != "" {
		status, ok, err := store.LoadStatus(focusTicketID)
		if err != nil {
			return "", runruntime.StatusSnapshot{}, false, []string{"status unavailable for " + focusTicketID + ": " + err.Error()}
		}
		return focusTicketID, status, ok, warnings
	}
	if cycle.CurrentTicketID != "" {
		status, ok, err := store.LoadStatus(cycle.CurrentTicketID)
		if err != nil {
			warnings = append(warnings, "cycle ticket status unavailable: "+err.Error())
		} else if ok {
			return cycle.CurrentTicketID, status, ok, warnings
		}
	}
	ticketIDs, err := store.ListRunTicketIDs()
	if err != nil {
		return "", runruntime.StatusSnapshot{}, false, []string{"runtime runs unavailable: " + err.Error()}
	}
	type candidate struct {
		ticketID string
		status   runruntime.StatusSnapshot
	}
	candidates := make([]candidate, 0, len(ticketIDs))
	for _, ticketID := range ticketIDs {
		status, ok, err := store.LoadStatus(ticketID)
		if err != nil {
			warnings = append(warnings, "status unavailable for "+ticketID+": "+err.Error())
			continue
		}
		if !ok {
			continue
		}
		if !status.Active {
			continue
		}
		candidates = append(candidates, candidate{ticketID: ticketID, status: status})
	}
	if len(candidates) == 0 {
		return "", runruntime.StatusSnapshot{}, false, warnings
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].status.Active != candidates[j].status.Active {
			return candidates[i].status.Active
		}
		return candidates[i].status.LastEventAt > candidates[j].status.LastEventAt
	})
	best := candidates[0]
	return best.ticketID, best.status, true, warnings
}

func RunWatchKeys() []string {
	return slices.Clone([]string{"j", "k", "enter", "l", "tab", "p", "s", "x", "r", "m", "?", "q"})
}

func (m RunWatchModel) runWatchKeyLegend() string {
	parts := []string{"l/tab toggle", "j/k scroll", "g top", "G follow", "h overview", "p ping", "s stop-after", "x hard-stop", "r refresh", "? help"}
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
