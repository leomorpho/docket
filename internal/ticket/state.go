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
	if from == to {
		return nil
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("cannot transition from %q to %q", from, to)
	}
	return nil
}
