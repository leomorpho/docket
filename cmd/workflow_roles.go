package cmd

import "github.com/leomorpho/docket/internal/ticket"

func preferredStateForRole(cfg *ticket.Config, role, fallback string) string {
	if cfg != nil {
		if state, ok := cfg.PrimaryStateWithRole(role); ok {
			return state
		}
	}
	return fallback
}

func nextStateForRole(cfg *ticket.Config, from ticket.State, role, fallback string) string {
	if cfg != nil {
		for _, next := range cfg.TransitionTargetsWithRole(string(from), role) {
			return next
		}
	}
	return preferredStateForRole(cfg, role, fallback)
}
