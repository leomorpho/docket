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
		if next := nextTransitionTowardRole(cfg, string(from), role); next != "" {
			return next
		}
	}
	return preferredStateForRole(cfg, role, fallback)
}

func nextTransitionTowardRole(cfg *ticket.Config, from, role string) string {
	if cfg == nil {
		return ""
	}
	for _, candidate := range cfg.ValidTransitions(from) {
		if candidate == "" {
			continue
		}
		if cfg.StateHasRole(candidate, role) {
			return candidate
		}
		visited := map[string]bool{from: true}
		if canReachRole(cfg, candidate, role, visited) {
			return candidate
		}
	}
	return ""
}

func canReachRole(cfg *ticket.Config, state, role string, visited map[string]bool) bool {
	if cfg == nil || state == "" {
		return false
	}
	if cfg.StateHasRole(state, role) {
		return true
	}
	if visited[state] {
		return false
	}
	visited[state] = true
	for _, next := range cfg.ValidTransitions(state) {
		if canReachRole(cfg, next, role, visited) {
			return true
		}
	}
	return false
}
