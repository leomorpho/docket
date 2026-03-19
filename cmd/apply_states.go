package cmd

import "github.com/leomorpho/docket/internal/ticket"

func applyAllowedStates(cfg *ticket.Config) map[string]struct{} {
	allowed := make(map[string]struct{}, len(cfg.States))
	for name := range cfg.States {
		allowed[name] = struct{}{}
	}
	return allowed
}
