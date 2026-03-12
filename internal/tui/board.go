package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

const virtualStateBlocked = "blocked"

type column struct {
	// state is the ticket state this column represents, or virtualStateBlocked
	// for the computed blocked column.
	state   string
	title   string
	tickets []*ticket.Ticket
}

type loadTicketsMsg struct {
	tickets []*ticket.Ticket
	err     error
}

type opMsg struct {
	status string
	err    error
}

type detailMsg struct {
	text string
	err  error
}

type BoardModel struct {
	repoRoot string
	backend  store.Backend
	actor    string
	cfg      *ticket.Config

	allTickets []*ticket.Ticket
	columns    []column

	// stateToColIdx maps state name → column index (excludes the blocked column).
	stateToColIdx map[string]int
	// blockedColIdx is the index of the virtual blocked column (-1 if absent).
	blockedColIdx int

	focusCol int
	focusRow int

	width  int
	height int

	showHelp      bool
	statusMessage string

	detailOpen    bool
	detailText    string
	detailLoading bool

	creatingTitle bool
	newTitle      string
}

// NewBoardModel creates a BoardModel. If config cannot be loaded it falls back
// to a minimal set of columns so the board still renders.
func NewBoardModel(repoRoot string, backend store.Backend, actor string) BoardModel {
	cfg, err := ticket.LoadConfig(repoRoot)
	if err != nil {
		// Fallback: minimal hard-coded columns so the board is usable even if
		// config is unavailable.
		cfg = ticket.DefaultConfig()
	}
	cols, stateToColIdx, blockedColIdx := buildColumnsFromConfig(cfg)
	return BoardModel{
		repoRoot:      repoRoot,
		backend:       backend,
		actor:         actor,
		cfg:           cfg,
		columns:       cols,
		stateToColIdx: stateToColIdx,
		blockedColIdx: blockedColIdx,
	}
}

// buildColumnsFromConfig creates the board column list from config.
// It returns the columns, a map from state name → column index, and the
// index of the virtual "BLOCKED" column.
func buildColumnsFromConfig(cfg *ticket.Config) ([]column, map[string]int, int) {
	ordered := cfg.ColumnOrder() // sorted by Column value

	// Find the last open-state column index in the ordered list.
	lastOpenIdx := -1
	for i, sc := range ordered {
		if sc.Open {
			lastOpenIdx = i
		}
	}

	var cols []column
	stateToColIdx := make(map[string]int, len(ordered)+1)

	// Find the state name for each StateConfig.
	nameFor := make(map[int]string, len(ordered)) // column value → state name
	for name, sc := range cfg.States {
		nameFor[sc.Column] = name
	}

	blockedInserted := false
	blockedColIdx := -1

	for i, sc := range ordered {
		stateName := nameFor[sc.Column]
		stateToColIdx[stateName] = len(cols)
		cols = append(cols, column{
			state: stateName,
			title: strings.ToUpper(sc.Label),
		})

		// Insert BLOCKED virtual column after the last open-state column.
		if i == lastOpenIdx && !blockedInserted {
			blockedColIdx = len(cols)
			cols = append(cols, column{state: virtualStateBlocked, title: "BLOCKED"})
			blockedInserted = true
		}
	}

	// If there were no open states, append blocked at the end.
	if !blockedInserted && len(cfg.States) > 0 {
		blockedColIdx = len(cols)
		cols = append(cols, column{state: virtualStateBlocked, title: "BLOCKED"})
	}

	return cols, stateToColIdx, blockedColIdx
}

func (m BoardModel) Init() tea.Cmd {
	return loadTicketsCmd(m.backend)
}

func (m BoardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case loadTicketsMsg:
		if msg.err != nil {
			m.statusMessage = "refresh failed: " + msg.err.Error()
			return m, nil
		}
		selectedID := m.selectedTicketID()
		m.allTickets = msg.tickets
		m.rebuildColumns(selectedID)
		if m.statusMessage == "" {
			m.statusMessage = "Loaded tickets"
		}
		return m, nil
	case opMsg:
		if msg.err != nil {
			m.statusMessage = msg.err.Error()
			return m, nil
		}
		m.statusMessage = msg.status
		return m, loadTicketsCmd(m.backend)
	case detailMsg:
		m.detailLoading = false
		if msg.err != nil {
			m.statusMessage = "detail failed: " + msg.err.Error()
			m.detailOpen = false
			m.detailText = ""
			return m, nil
		}
		m.detailOpen = true
		m.detailText = msg.text
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m BoardModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.creatingTitle {
		switch key {
		case "esc":
			m.creatingTitle = false
			m.newTitle = ""
			m.statusMessage = "Create canceled"
			return m, nil
		case "enter":
			title := strings.TrimSpace(m.newTitle)
			if title == "" {
				m.statusMessage = "title cannot be empty"
				return m, nil
			}
			m.creatingTitle = false
			m.newTitle = ""
			return m, createTicketCmd(m.backend, m.actor, title, m.cfg)
		case "backspace", "ctrl+h":
			m.newTitle = dropLastRune(m.newTitle)
			return m, nil
		default:
			if len(key) == 1 && key >= " " {
				m.newTitle += key
			}
			return m, nil
		}
	}

	if m.detailOpen {
		switch key {
		case "esc", "q":
			m.detailOpen = false
			m.detailText = ""
			m.statusMessage = "Back to board"
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "r":
		m.statusMessage = "Refreshing..."
		return m, loadTicketsCmd(m.backend)
	case "n":
		m.creatingTitle = true
		m.newTitle = ""
		m.statusMessage = "Enter title for new ticket"
		return m, nil
	case "enter":
		t := m.selectedTicket()
		if t == nil {
			m.statusMessage = "No ticket selected"
			return m, nil
		}
		m.detailLoading = true
		m.statusMessage = "Loading detail..."
		return m, loadDetailCmd(m.backend, t.ID)
	case "left":
		return m, m.moveStateCmd(-1)
	case "right":
		return m, m.moveStateCmd(1)
	case "up":
		return m, m.reorderPriorityCmd(-1)
	case "down":
		return m, m.reorderPriorityCmd(1)
	}

	return m, nil
}

func (m BoardModel) View() string {
	if m.detailOpen {
		return m.viewDetail()
	}

	board := m.viewColumns()

	if m.creatingTitle {
		board += "\n\nNew ticket title: " + m.newTitle + "_\n(enter to create, esc to cancel)"
	}

	status := "\n\n[←/→] move state  [↑/↓] reprioritize  [enter] view  [n] new  [r] refresh  [?] help  [q] quit"
	if m.statusMessage != "" {
		status += "\n" + m.statusMessage
	}

	if m.showHelp {
		status += "\n\nHelp: BLOCKED is computed from blocked_by and is read-only."
	}

	return board + status
}

func (m BoardModel) viewDetail() string {
	if m.detailLoading {
		return "Loading ticket detail..."
	}

	body := m.detailText
	if body == "" {
		body = "No detail available"
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)

	return style.Render(body) + "\n\n[esc] back  [q] back"
}

func (m BoardModel) viewColumns() string {
	count := len(m.columns)
	colWidth := 24
	if m.width > 0 {
		candidate := (m.width - (count-1)*1) / count
		if candidate > colWidth {
			colWidth = candidate
		}
		if colWidth < 18 {
			colWidth = 18
		}
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	colStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(colWidth).Padding(0, 1)
	focusedColStyle := colStyle.Copy().BorderForeground(lipgloss.Color("212"))
	itemStyle := lipgloss.NewStyle()
	focusedItemStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	rendered := make([]string, 0, len(m.columns))
	for ci, col := range m.columns {
		var lines []string
		lines = append(lines, headerStyle.Render(col.title))

		if len(col.tickets) == 0 {
			lines = append(lines, "")
		}

		for ri, t := range col.tickets {
			label := fmt.Sprintf("%s  P%d", t.ID, t.Priority)
			title := truncate(t.Title, colWidth-4)
			entry := label + "\n" + title
			if ci == m.focusCol && ri == m.focusRow {
				entry = focusedItemStyle.Render("▶ " + entry)
			} else {
				entry = itemStyle.Render("  " + entry)
			}
			lines = append(lines, entry)
		}

		body := strings.Join(lines, "\n")
		if ci == m.focusCol {
			rendered = append(rendered, focusedColStyle.Render(body))
		} else {
			rendered = append(rendered, colStyle.Render(body))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m BoardModel) moveStateCmd(delta int) tea.Cmd {
	if m.focusCol < 0 || m.focusCol >= len(m.columns) {
		return nil
	}
	if m.columns[m.focusCol].state == virtualStateBlocked {
		return func() tea.Msg {
			return opMsg{err: fmt.Errorf("cannot move tickets from BLOCKED column")}
		}
	}
	selected := m.selectedTicket()
	if selected == nil {
		return nil
	}

	target, err := m.targetState(selected.State, delta)
	if err != nil {
		return func() tea.Msg {
			return opMsg{err: err}
		}
	}

	return updateStateCmd(m.backend, selected.ID, target, m.cfg)
}

func (m BoardModel) reorderPriorityCmd(delta int) tea.Cmd {
	if m.focusCol < 0 || m.focusCol >= len(m.columns) {
		return nil
	}
	if m.columns[m.focusCol].state == virtualStateBlocked {
		return func() tea.Msg {
			return opMsg{err: fmt.Errorf("cannot reprioritize in BLOCKED column")}
		}
	}

	col := m.columns[m.focusCol]
	if len(col.tickets) == 0 {
		return nil
	}

	targetRow := m.focusRow + delta
	if targetRow < 0 || targetRow >= len(col.tickets) {
		return nil
	}

	first := col.tickets[m.focusRow]
	second := col.tickets[targetRow]

	m.focusRow = targetRow
	return swapPriorityCmd(m.backend, first.ID, second.ID)
}

func (m *BoardModel) rebuildColumns(selectedID string) {
	// Clear all column ticket lists.
	for i := range m.columns {
		m.columns[i].tickets = nil
	}

	for _, t := range m.allTickets {
		// Assign to the matching state column.
		if colIdx, ok := m.stateToColIdx[string(t.State)]; ok {
			m.columns[colIdx].tickets = append(m.columns[colIdx].tickets, t)
		}

		// Always add blocked tickets to the virtual blocked column.
		if len(t.BlockedBy) > 0 && m.blockedColIdx >= 0 {
			m.columns[m.blockedColIdx].tickets = append(m.columns[m.blockedColIdx].tickets, t)
		}
	}

	for i := range m.columns {
		sort.SliceStable(m.columns[i].tickets, func(a, b int) bool {
			left := m.columns[i].tickets[a]
			right := m.columns[i].tickets[b]
			if left.Priority != right.Priority {
				return left.Priority < right.Priority
			}
			return left.CreatedAt.Before(right.CreatedAt)
		})
	}

	m.clampFocus(selectedID)
}

func (m *BoardModel) clampFocus(selectedID string) {
	if selectedID != "" {
		for ci := range m.columns {
			for ri, t := range m.columns[ci].tickets {
				if t.ID == selectedID {
					m.focusCol = ci
					m.focusRow = ri
					return
				}
			}
		}
	}

	if m.focusCol < 0 {
		m.focusCol = 0
	}
	if m.focusCol >= len(m.columns) {
		m.focusCol = len(m.columns) - 1
	}

	if len(m.columns[m.focusCol].tickets) == 0 {
		for ci := range m.columns {
			if len(m.columns[ci].tickets) > 0 {
				m.focusCol = ci
				m.focusRow = 0
				return
			}
		}
		m.focusRow = 0
		return
	}

	if m.focusRow < 0 {
		m.focusRow = 0
	}
	if m.focusRow >= len(m.columns[m.focusCol].tickets) {
		m.focusRow = len(m.columns[m.focusCol].tickets) - 1
	}
}

func (m BoardModel) selectedTicket() *ticket.Ticket {
	if m.focusCol < 0 || m.focusCol >= len(m.columns) {
		return nil
	}
	col := m.columns[m.focusCol]
	if m.focusRow < 0 || m.focusRow >= len(col.tickets) {
		return nil
	}
	return col.tickets[m.focusRow]
}

func (m BoardModel) selectedTicketID() string {
	t := m.selectedTicket()
	if t == nil {
		return ""
	}
	return t.ID
}

// targetState returns the next state when moving left (delta=-1) or right
// (delta=+1) on the board, using the config column order.
func (m BoardModel) targetState(current ticket.State, delta int) (ticket.State, error) {
	// Build ordered list of state names (excluding virtual blocked column).
	var order []string
	for _, col := range m.columns {
		if col.state != virtualStateBlocked {
			order = append(order, col.state)
		}
	}

	idx := -1
	for i, st := range order {
		if ticket.State(st) == current {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", fmt.Errorf("cannot transition unknown state: %s", current)
	}

	targetIdx := idx + delta
	if targetIdx < 0 {
		return "", fmt.Errorf("cannot transition left from %s", current)
	}
	if targetIdx >= len(order) {
		return "", fmt.Errorf("cannot transition right from %s", current)
	}

	target := ticket.State(order[targetIdx])
	if err := ticket.ValidateTransition(m.cfg, current, target); err != nil {
		return "", err
	}

	return target, nil
}

func loadTicketsCmd(backend store.Backend) tea.Cmd {
	return func() tea.Msg {
		tickets, err := backend.ListTickets(context.Background(), store.Filter{IncludeArchived: false})
		return loadTicketsMsg{tickets: tickets, err: err}
	}
}

func loadDetailCmd(backend store.Backend, id string) tea.Cmd {
	return func() tea.Msg {
		t, err := backend.GetTicket(context.Background(), id)
		if err != nil {
			return detailMsg{err: err}
		}
		if t == nil {
			return detailMsg{err: fmt.Errorf("ticket %s not found", id)}
		}
		return detailMsg{text: formatTicketDetail(t)}
	}
}

func updateStateCmd(backend store.Backend, id string, next ticket.State, cfg *ticket.Config) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		t, err := backend.GetTicket(ctx, id)
		if err != nil {
			return opMsg{err: err}
		}
		if t == nil {
			return opMsg{err: fmt.Errorf("ticket %s not found", id)}
		}

		if err := ticket.ValidateTransition(cfg, t.State, next); err != nil {
			return opMsg{err: err}
		}

		t.State = next
		t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
		if err := backend.UpdateTicket(ctx, t); err != nil {
			return opMsg{err: err}
		}

		return opMsg{status: fmt.Sprintf("Moved %s to %s", t.ID, next)}
	}
}

func swapPriorityCmd(backend store.Backend, firstID, secondID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		first, err := backend.GetTicket(ctx, firstID)
		if err != nil {
			return opMsg{err: err}
		}
		if first == nil {
			return opMsg{err: fmt.Errorf("ticket %s not found", firstID)}
		}

		second, err := backend.GetTicket(ctx, secondID)
		if err != nil {
			return opMsg{err: err}
		}
		if second == nil {
			return opMsg{err: fmt.Errorf("ticket %s not found", secondID)}
		}

		first.Priority, second.Priority = second.Priority, first.Priority
		now := time.Now().UTC().Truncate(time.Second)
		first.UpdatedAt = now
		second.UpdatedAt = now

		if err := backend.UpdateTicket(ctx, first); err != nil {
			return opMsg{err: err}
		}
		if err := backend.UpdateTicket(ctx, second); err != nil {
			return opMsg{err: err}
		}

		return opMsg{status: fmt.Sprintf("Reordered %s and %s", first.ID, second.ID)}
	}
}

func createTicketCmd(backend store.Backend, actor, title string, cfg *ticket.Config) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		id, seq, err := backend.NextID(ctx)
		if err != nil {
			return opMsg{err: err}
		}

		now := time.Now().UTC().Truncate(time.Second)
		t := &ticket.Ticket{
			ID:          id,
			Seq:         seq,
			Title:       title,
			Description: "",
			Priority:    cfg.DefaultPriority,
			State:       ticket.State(cfg.DefaultState),
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   actor,
		}

		if err := backend.CreateTicket(ctx, t); err != nil {
			return opMsg{err: err}
		}

		return opMsg{status: fmt.Sprintf("Created %s in %s", t.ID, cfg.DefaultState)}
	}
}

func truncate(s string, width int) string {
	r := []rune(s)
	if width <= 0 || len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

func dropLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}
