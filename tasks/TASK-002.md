# TASK-002: Core data types

## Status
`[ ]` not started

## Depends on
TASK-001

## What this is
Define the core Go structs for tickets, states, plan steps, and acceptance criteria
in `internal/ticket/`. This is the schema layer — everything else builds on these types.

## Key design decisions (read ARCHITECTURE.md before changing anything here)
- `blocked` is NOT a State value — it is computed from `blocked_by` being non-empty
  with unresolved (non-done, non-archived) tickets
- All times are `time.Time` in Go, marshalled as RFC3339 strings in YAML/JSON
- Actor format: `human:username` or `agent:model-id` (e.g. `agent:claude-sonnet-4-6`)
- State is stored as a string in the markdown frontmatter

## Files to create

### `internal/ticket/ticket.go`

```go
package ticket

import "time"

type State string

const (
    StateBacklog    State = "backlog"
    StateTodo       State = "todo"
    StateInProgress State = "in-progress"
    StateInReview   State = "in-review"
    StateDone       State = "done"
    StateArchived   State = "archived"
)

var ValidStates = []State{
    StateBacklog, StateTodo, StateInProgress,
    StateInReview, StateDone, StateArchived,
}

func IsValidState(s State) bool {
    for _, v := range ValidStates {
        if v == s {
            return true
        }
    }
    return false
}

type PlanStep struct {
    Description string `yaml:"description"`
    Status      string `yaml:"status"` // "pending" | "done"
    Notes       string `yaml:"notes,omitempty"`
}

type AcceptanceCriterion struct {
    Description string `yaml:"description"`
    Done        bool   `yaml:"done"`
    Evidence    string `yaml:"evidence,omitempty"`
}

type Ticket struct {
    // Frontmatter fields
    ID            string    `yaml:"id"`
    Seq           int       `yaml:"seq"`
    State         State     `yaml:"state"`
    Priority      int       `yaml:"priority"`
    Labels        []string  `yaml:"labels,omitempty"`
    BlockedBy     []string  `yaml:"blocked_by,omitempty"`
    Blocks        []string  `yaml:"blocks,omitempty"`
    LinkedCommits []string  `yaml:"linked_commits,omitempty"`
    CreatedAt     time.Time `yaml:"created_at"`
    UpdatedAt     time.Time `yaml:"updated_at"`
    CreatedBy     string    `yaml:"created_by"`

    // Parsed from markdown body
    Title       string
    Description string
    Plan        []PlanStep
    AC          []AcceptanceCriterion
    Comments    []Comment
    Handoff     string
}

type Comment struct {
    At     time.Time
    Author string
    Body   string
}

// IsBlocked returns true if this ticket has unresolved blockers.
func (t *Ticket) IsBlocked() bool {
    return len(t.BlockedBy) > 0
}

// ACComplete returns true if all AC items are done (or there are none).
func (t *Ticket) ACComplete() bool {
    for _, ac := range t.AC {
        if !ac.Done {
            return false
        }
    }
    return true
}
```

### `internal/ticket/state.go`

```go
package ticket

import "fmt"

var validTransitions = map[State][]State{
    StateBacklog:    {StateTodo, StateArchived},
    StateTodo:       {StateInProgress, StateBacklog, StateArchived},
    StateInProgress: {StateInReview, StateTodo, StateArchived},
    StateInReview:   {StateDone, StateInProgress, StateArchived},
    StateDone:       {StateArchived, StateInProgress},
    StateArchived:   {StateBacklog},
}

func CanTransition(from, to State) bool {
    for _, s := range validTransitions[from] {
        if s == to {
            return true
        }
    }
    return false
}

func ValidateTransition(from, to State) error {
    if !CanTransition(from, to) {
        return fmt.Errorf("cannot transition from %q to %q", from, to)
    }
    return nil
}
```

## Acceptance criteria
- [ ] All structs compile with no errors
- [ ] `IsBlocked()` returns true only when `BlockedBy` is non-empty
- [ ] `ACComplete()` returns false when any AC has `Done: false`
- [ ] `ACComplete()` returns true when slice is empty
- [ ] `ValidateTransition(StateBacklog, StateTodo)` returns nil
- [ ] `ValidateTransition(StateBacklog, StateDone)` returns error
- [ ] `IsValidState("in-progress")` returns true
- [ ] `IsValidState("blocked")` returns false — blocked is computed, not a state
- [ ] `go test ./internal/ticket/...` passes

## Notes for LLM
- Do not add persistence here — that is TASK-004
- Do not add ID generation here — that is TASK-003
- Write a `state_test.go` with a table-driven test covering all valid and a few invalid transitions
- `Comment` struct is for parsed comments from the markdown body, not for persistence
