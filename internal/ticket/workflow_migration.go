package ticket

import "reflect"

var legacyWorkflowStateAliases = map[string]string{
	"backlog":     "draft",
	"todo":        "ready",
	"in-progress": "running",
	"in-review":   "validated",
	"done":        "validated",
}

// MigrateWorkflowStateName maps legacy workflow state names to their
// north-star equivalents and returns the input unchanged otherwise.
func MigrateWorkflowStateName(state string) string {
	if mapped, ok := legacyWorkflowStateAliases[state]; ok {
		return mapped
	}
	return state
}

// MigrateConfigToNorthStar returns a config that uses the north-star workflow
// for canonical states while preserving any non-legacy custom states.
func MigrateConfigToNorthStar(cfg *Config) (*Config, bool) {
	if cfg == nil {
		return nil, false
	}

	migrated := cloneConfig(cfg)
	defaultCfg := DefaultConfig()
	newStates := make(map[string]StateConfig)

	if usesNorthStarWorkflowStateNames(cfg) {
		for _, name := range canonicalNorthStarStateNames() {
			state, ok := cfg.States[name]
			if !ok {
				state = defaultCfg.States[name]
			}
			newStates[name] = cloneStateConfig(state)
		}
	} else {
		for _, name := range canonicalNorthStarStateNames() {
			newStates[name] = cloneStateConfig(defaultCfg.States[name])
		}
	}

	for name, state := range cfg.States {
		if isLegacyWorkflowStateName(name) || isCanonicalNorthStarStateName(name) {
			continue
		}
		cloned := cloneStateConfig(state)
		cloned.Next = migrateWorkflowTransitionNames(cloned.Next)
		newStates[name] = cloned
	}

	for name, state := range newStates {
		state.Next = migrateWorkflowTransitionNames(state.Next)
		newStates[name] = state
	}

	migrated.States = newStates
	migrated.DefaultState = MigrateWorkflowStateName(cfg.DefaultState)
	migrated.Workflow = workflowFromStates(newStates)

	changed := migrated.DefaultState != cfg.DefaultState || !reflect.DeepEqual(migrated.States, cfg.States)
	return migrated, changed
}

func canonicalNorthStarStateNames() []string {
	return []string{"draft", "ready", "running", "validated", "archived"}
}

func usesNorthStarWorkflowStateNames(cfg *Config) bool {
	if cfg == nil {
		return false
	}
	for _, name := range canonicalNorthStarStateNames() {
		if _, ok := cfg.States[name]; !ok {
			return false
		}
	}
	return true
}

func isCanonicalNorthStarStateName(name string) bool {
	for _, candidate := range canonicalNorthStarStateNames() {
		if name == candidate {
			return true
		}
	}
	return false
}

func isLegacyWorkflowStateName(name string) bool {
	_, ok := legacyWorkflowStateAliases[name]
	return ok
}

func migrateWorkflowTransitionNames(next []string) []string {
	if len(next) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(next))
	migrated := make([]string, 0, len(next))
	for _, candidate := range next {
		name := MigrateWorkflowStateName(candidate)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		migrated = append(migrated, name)
	}
	return migrated
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.States = make(map[string]StateConfig, len(cfg.States))
	for name, state := range cfg.States {
		cloned.States[name] = cloneStateConfig(state)
	}
	cloned.Workflow = cloneWorkflowConfig(cfg.Workflow)
	cloned.Labels = append([]string(nil), cfg.Labels...)
	cloned.HandoffSections = append([]string(nil), cfg.HandoffSections...)
	cloned.Backends = cloneBackendConfigs(cfg.Backends)
	return &cloned
}

func cloneStateConfig(state StateConfig) StateConfig {
	state.Next = append([]string(nil), state.Next...)
	state.Roles = append([]string(nil), state.Roles...)
	return state
}

func cloneBackendConfigs(in map[string]BackendConfig) map[string]BackendConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]BackendConfig, len(in))
	for name, cfg := range in {
		if cfg == nil {
			out[name] = nil
			continue
		}
		clone := make(BackendConfig, len(cfg))
		for key, value := range cfg {
			clone[key] = value
		}
		out[name] = clone
	}
	return out
}
