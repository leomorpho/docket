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
	ID            string    `yaml:"id" json:"id"`
	Seq           int       `yaml:"seq" json:"seq"`
	State         State     `yaml:"state" json:"state"`
	Priority      int       `yaml:"priority" json:"priority"`
	Labels        []string  `yaml:"labels,omitempty" json:"labels,omitempty"`
	BlockedBy     []string  `yaml:"blocked_by,omitempty" json:"blocked_by,omitempty"`
	Blocks        []string  `yaml:"blocks,omitempty" json:"blocks,omitempty"`
	LinkedCommits []string  `yaml:"linked_commits,omitempty" json:"linked_commits,omitempty"`
	CreatedAt     time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt     time.Time `yaml:"updated_at" json:"updated_at"`
	CreatedBy     string    `yaml:"created_by" json:"created_by"`

	// Parsed from markdown body
	Title       string                `json:"title"`
	Description string                `json:"description,omitempty"`
	Plan        []PlanStep            `json:"plan,omitempty"`
	AC          []AcceptanceCriterion `json:"ac,omitempty"`
	Comments    []Comment             `json:"comments,omitempty"`
	Handoff     string                `json:"handoff,omitempty"`
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
