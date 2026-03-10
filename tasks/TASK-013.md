# TASK-013: `docket board` — bubbletea TUI kanban

## Status
`[x]` done

## Depends on
TASK-012 (SQLite index for fast column queries)

## What this is
An interactive terminal kanban board. The primary human interaction surface for managing
ticket state and priority.

## Layout
```
┌─ BACKLOG ────┐ ┌─ TODO ───────┐ ┌─ IN PROGRESS ┐ ┌─ BLOCKED ────┐ ┌─ IN REVIEW ──┐ ┌─ DONE ───────┐
│▶TKT-005  P2  │ │  TKT-003  P1 │ │  TKT-001  P1 │ │  TKT-002     │ │              │ │  TKT-000     │
│  Auth refact │ │  Fix login   │ │  Add MCP srv │ │  ← TKT-001   │ │              │ │  Init setup  │
│              │ │  TKT-006  P2 │ │              │ │              │ │              │ │              │
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘

[←/→] move state  [↑/↓] reprioritize  [enter] view  [n] new  [r] refresh  [q] quit
```

BLOCKED column is computed — shows tickets with unresolved `blocked_by`. Not a manual state.

## Keyboard bindings
| Key | Action |
|-----|--------|
| `←` / `→` | Move focused ticket to previous/next state |
| `↑` / `↓` | Move ticket up/down within column (reorder priority) |
| `enter` | Open ticket detail view (shows `docket show` output) |
| `n` | Prompt for new ticket title, create in BACKLOG |
| `r` | Refresh from disk |
| `q` / `ctrl+c` | Quit |
| `?` | Toggle help overlay |

## State transitions via board
Moving a ticket left/right only works for valid transitions (per state machine in TASK-002).
Invalid moves show a brief error message at the bottom of the screen.

Priority reordering: `↑` swaps the ticket with the one above it in the same column.
Calls `docket update TKT-NNN --priority <new>` internally (re-numbers affected tickets).

## Files to create

### `internal/tui/board.go`
Bubbletea `Model` implementing the kanban board.
- `Init()`: load tickets via store, build column data
- `Update(msg)`: handle keyboard, call store on state changes
- `View()`: render columns with lipgloss styling

### `internal/tui/detail.go`
Ticket detail overlay shown on `enter`. Renders `docket show --format human` output.
Press `esc` or `q` to return to board.

### `cmd/board.go`
```go
// Launches the bubbletea program
func runBoard(repoRoot string, backend store.Backend) error
```

## Dependencies to add
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
```

## Acceptance criteria
- [x] `docket board` renders columns with tickets sorted by priority
- [x] `→` on TKT-001 in `backlog` moves it to `todo` and updates the markdown file
- [x] `→` on a ticket in `done` shows error "cannot transition from done to archived via board" (or similar)
- [x] `↑`/`↓` reorders priority within a column and persists the change
- [x] `enter` opens detail view, `esc` returns to board
- [x] `n` prompts for title, creates ticket, adds to BACKLOG column
- [x] `r` refreshes ticket data from disk
- [x] `q` exits cleanly with no terminal state corruption
- [x] BLOCKED column shows tickets with non-empty `blocked_by` regardless of manual state

## Notes for LLM
- Use lipgloss for column borders, colors, and the focused ticket highlight
- The board calls `backend.UpdateTicket` on state/priority changes — no direct file writes
- Keep the TUI model thin: load data from store, delegate mutations back to store
- Prioritize correctness over visual polish — a working board > a pretty broken one
