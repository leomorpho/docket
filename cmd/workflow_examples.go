package cmd

import "github.com/leomorpho/docket/internal/ticket"

func activeWorkflowState(cfg *ticket.Config) string {
	return preferredStateForRole(cfg, "active", "in-progress")
}

func reviewWorkflowState(cfg *ticket.Config) string {
	return preferredStateForRole(cfg, "review", "in-review")
}

func completedWorkflowState(cfg *ticket.Config) string {
	return preferredStateForRole(cfg, "completed", "done")
}
