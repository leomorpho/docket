package ticket

import "testing"

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		from     State
		to       State
		expected bool // true if nil error expected
	}{
		// Valid transitions
		{StateBacklog, StateTodo, true},
		{StateBacklog, StateArchived, true},
		{StateTodo, StateInProgress, true},
		{StateTodo, StateBacklog, true},
		{StateTodo, StateArchived, true},
		{StateInProgress, StateInReview, true},
		{StateInProgress, StateTodo, true},
		{StateInProgress, StateArchived, true},
		{StateInReview, StateDone, true},
		{StateInReview, StateInProgress, true},
		{StateInReview, StateArchived, true},
		{StateDone, StateArchived, true},
		{StateDone, StateInProgress, true},
		{StateArchived, StateBacklog, true},
		// Self transitions
		{StateBacklog, StateBacklog, true},
		{StateTodo, StateTodo, true},
		// Invalid transitions
		{StateBacklog, StateDone, false},
		{StateBacklog, StateInProgress, false},
		{StateDone, StateTodo, false},
		{StateArchived, StateDone, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if (err == nil) != tt.expected {
				t.Errorf("ValidateTransition(%q, %q) error = %v, expected nil: %v", tt.from, tt.to, err, tt.expected)
			}
		})
	}
}
