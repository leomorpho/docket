package ticket

import "fmt"

// ValidateTransition checks whether transitioning from → to is allowed by cfg.
func ValidateTransition(cfg *Config, from, to State) error {
	if from == to {
		return nil
	}
	for _, next := range cfg.ValidTransitions(string(from)) {
		if string(to) == next {
			return nil
		}
	}
	return fmt.Errorf("cannot transition from %q to %q", from, to)
}
