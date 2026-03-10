package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

type boardColumn int

const (
	colBacklog boardColumn = iota
	colTodo
	colInProgress
	colBlocked
	colInReview
	colDone
)

type column struct {
	kind    boardColumn
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

	allTickets []*ticket.Ticket
	columns    []column

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

func NewBoardModel(repoRoot string, backend store.Backend, actor string) BoardModel {
	return BoardModel{
		repoRoot: repoRoot,
		backend:  backend,
		actor:    actor,
		columns: []column{
			{kind: colBacklog, title: "BACKLOG"},
			{kind: colTodo, title: "TODO"},
			{kind: colInProgress, title: "IN PROGRESS"},
			{kind: colBlocked, title: "BLOCKED"},
			{kind: colInReview, title: "IN REVIEW"},
			{kind: colDone, title: "DONE"},
		},
	}
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
			return m, createTicketCmd(m.backend, m.actor, title)
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
	col := m.columns[m.focusCol]
	if col.kind == colBlocked {
		return func() tea.Msg {
			return opMsg{err: fmt.Errorf("cannot move tickets from BLOCKED column")}
		}
	}
	selected := m.selectedTicket()
	if selected == nil {
		return nil
	}

	target, err := targetState(selected.State, delta)
	if err != nil {
		return func() tea.Msg {
			return opMsg{err: err}
		}
	}

	return updateStateCmd(m.backend, selected.ID, target)
}

func (m BoardModel) reorderPriorityCmd(delta int) tea.Cmd {
	if m.focusCol < 0 || m.focusCol >= len(m.columns) {
		return nil
	}
	col := m.columns[m.focusCol]
	if col.kind == colBlocked {
		return func() tea.Msg {
			return opMsg{err: fmt.Errorf("cannot reprioritize in BLOCKED column")}
		}
	}

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
	for i := range m.columns {
		m.columns[i].tickets = nil
	}

	for _, t := range m.allTickets {
		switch t.State {
		case ticket.StateBacklog:
			m.columns[colBacklog].tickets = append(m.columns[colBacklog].tickets, t)
		case ticket.StateTodo:
			m.columns[colTodo].tickets = append(m.columns[colTodo].tickets, t)
		case ticket.StateInProgress:
			m.columns[colInProgress].tickets = append(m.columns[colInProgress].tickets, t)
		case ticket.StateInReview:
			m.columns[colInReview].tickets = append(m.columns[colInReview].tickets, t)
		case ticket.StateDone:
			m.columns[colDone].tickets = append(m.columns[colDone].tickets, t)
		}

		if len(t.BlockedBy) > 0 {
			m.columns[colBlocked].tickets = append(m.columns[colBlocked].tickets, t)
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

func targetState(current ticket.State, delta int) (ticket.State, error) {
	order := []ticket.State{
		ticket.StateBacklog,
		ticket.StateTodo,
		ticket.StateInProgress,
		ticket.StateInReview,
		ticket.StateDone,
	}

	idx := -1
	for i, st := range order {
		if st == current {
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
		if current == ticket.StateDone && delta > 0 {
			return "", fmt.Errorf("cannot transition from done to archived via board")
		}
		return "", fmt.Errorf("cannot transition right from %s", current)
	}

	target := order[targetIdx]
	if err := ticket.ValidateTransition(current, target); err != nil {
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

func updateStateCmd(backend store.Backend, id string, next ticket.State) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		t, err := backend.GetTicket(ctx, id)
		if err != nil {
			return opMsg{err: err}
		}
		if t == nil {
			return opMsg{err: fmt.Errorf("ticket %s not found", id)}
		}

		if err := ticket.ValidateTransition(t.State, next); err != nil {
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

func createTicketCmd(backend store.Backend, actor, title string) tea.Cmd {
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
			Priority:    10,
			State:       ticket.StateBacklog,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   actor,
		}

		if err := backend.CreateTicket(ctx, t); err != nil {
			return opMsg{err: err}
		}

		return opMsg{status: fmt.Sprintf("Created %s in backlog", t.ID)}
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
