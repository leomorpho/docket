package ticket

import "testing"

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		name      string
		blockedBy []string
		expected  bool
	}{
		{"not blocked", []string{}, false},
		{"nil blocked", nil, false},
		{"blocked", []string{"TKT-1"}, true},
		{"multiple blockers", []string{"TKT-1", "TKT-2"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tick := &Ticket{BlockedBy: tt.blockedBy}
			if got := tick.IsBlocked(); got != tt.expected {
				t.Errorf("IsBlocked() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestACComplete(t *testing.T) {
	tests := []struct {
		name     string
		ac       []AcceptanceCriterion
		expected bool
	}{
		{"no AC", []AcceptanceCriterion{}, true}, // empty slice
		{"all AC done", []AcceptanceCriterion{{Done: true}, {Done: true}}, true},
		{"some AC pending", []AcceptanceCriterion{{Done: true}, {Done: false}}, false},
		{"all AC pending", []AcceptanceCriterion{{Done: false}, {Done: false}}, false},
		{"nil AC", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tick := &Ticket{AC: tt.ac}
			if got := tick.ACComplete(); got != tt.expected {
				t.Errorf("ACComplete() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsValidState(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		state    string
		expected bool
	}{
		{"backlog", true},
		{"todo", true},
		{"in-progress", true},
		{"in-review", true},
		{"done", true},
		{"archived", true},
		{"blocked", false},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := cfg.IsValidState(tt.state); got != tt.expected {
				t.Errorf("cfg.IsValidState(%q) = %v, want %v", tt.state, got, tt.expected)
			}
		})
	}
}
