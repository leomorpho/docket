package hooks

import (
	"fmt"
)

type Event string

const (
	EventRunStart   Event = "run.start"
	EventReviewGate Event = "ticket.review"
	EventQAGate     Event = "ticket.qa"
	EventPrivileged Event = "ticket.privileged"
)

type Mode string

const (
	ModeAdvisory    Mode = "advisory"
	ModeEnforcement Mode = "enforcement"
)

type Context struct {
	Repo                 string
	TicketID             string
	Actor                string
	TargetState          string
	ManagedRun           bool
	WorktreePath         string
	Branch               string
	RunStartedAt         string
	PrivilegedAuthorized bool
}

type HookFunc func(Context) error

type Registration struct {
	Name  string
	Event Event
	Mode  Mode
	Run   HookFunc
}

type Manager struct {
	byEvent map[Event][]Registration
}

func NewManager() *Manager {
	return &Manager{byEvent: map[Event][]Registration{}}
}

func (m *Manager) Register(reg Registration) {
	m.byEvent[reg.Event] = append(m.byEvent[reg.Event], reg)
}

func (m *Manager) Run(event Event, ctx Context) ([]string, error) {
	var advisory []string
	for _, reg := range m.byEvent[event] {
		if reg.Run == nil {
			continue
		}
		if err := reg.Run(ctx); err != nil {
			if reg.Mode == ModeEnforcement {
				return advisory, fmt.Errorf("%s: %w", reg.Name, err)
			}
			advisory = append(advisory, fmt.Sprintf("%s: %v", reg.Name, err))
		}
	}
	return advisory, nil
}
