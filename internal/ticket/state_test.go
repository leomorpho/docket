package ticket

import "testing"

func TestValidateTransition(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		from     State
		to       State
		expected bool // true if nil error expected
	}{
		// Valid transitions (from DefaultConfig)
		{"backlog", "todo", true},
		{"backlog", "archived", true},
		{"todo", "in-progress", true},
		{"todo", "backlog", true},
		{"todo", "archived", true},
		{"in-progress", "in-review", true},
		{"in-progress", "todo", true},
		{"in-progress", "archived", true},
		{"in-review", "done", true},
		{"in-review", "in-progress", true},
		{"in-review", "archived", true},
		{"done", "archived", true},
		{"done", "in-progress", true},
		{"archived", "backlog", true},
		// Self transitions
		{"backlog", "backlog", true},
		{"todo", "todo", true},
		// Invalid transitions
		{"backlog", "done", false},
		{"backlog", "in-progress", false},
		{"done", "todo", false},
		{"archived", "done", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			err := ValidateTransition(cfg, tt.from, tt.to)
			if (err == nil) != tt.expected {
				t.Errorf("ValidateTransition(%q, %q) error = %v, expected nil: %v", tt.from, tt.to, err, tt.expected)
			}
		})
	}
}
