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
		{"draft", "ready", true},
		{"draft", "archived", true},
		{"ready", "running", true},
		{"ready", "draft", true},
		{"ready", "archived", true},
		{"running", "validated", true},
		{"running", "ready", true},
		{"running", "draft", true},
		{"running", "archived", true},
		{"validated", "archived", true},
		{"validated", "running", true},
		{"archived", "draft", true},
		// Self transitions
		{"draft", "draft", true},
		{"ready", "ready", true},
		// Invalid transitions
		{"draft", "validated", false},
		{"draft", "running", false},
		{"validated", "ready", false},
		{"archived", "validated", false},
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
