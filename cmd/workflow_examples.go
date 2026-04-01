package cmd

import "github.com/leomorpho/docket/internal/ticket"

func activeWorkflowState(cfg *ticket.Config) string {
	return preferredStateForRole(cfg, "active", "running")
}

func reviewWorkflowState(cfg *ticket.Config) string {
	if cfg != nil {
		if state, ok := cfg.PrimaryStateWithRole("review"); ok {
			return state
		}
		if state, ok := cfg.PrimaryStateWithRole("completed"); ok {
			return state
		}
	}
	return "validated"
}

func completedWorkflowState(cfg *ticket.Config) string {
	return preferredStateForRole(cfg, "completed", "validated")
}
