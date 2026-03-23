package ticket

// WorkflowEvaluator resolves workflow semantics from config for state transitions,
// semantic roles, and dependency blocking behavior.
type WorkflowEvaluator struct {
	cfg *Config
}

func NewWorkflowEvaluator(cfg *Config) WorkflowEvaluator {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return WorkflowEvaluator{cfg: cfg}
}

func (e WorkflowEvaluator) CanTransition(from, to State) bool {
	return ValidateTransition(e.cfg, from, to) == nil
}

func (e WorkflowEvaluator) StateHasRole(state State, role string) bool {
	return e.cfg.StateHasRole(string(state), role)
}

func (e WorkflowEvaluator) IsStartable(state State) bool {
	sc, ok := e.cfg.States[string(state)]
	return ok && sc.Startable
}

func (e WorkflowEvaluator) IsReviewable(state State) bool {
	sc, ok := e.cfg.States[string(state)]
	return ok && sc.Reviewable
}

func (e WorkflowEvaluator) BlocksDependents(state State) bool {
	return e.cfg.BlocksDependents(state)
}

func (e WorkflowEvaluator) StartableStates() []string {
	return e.cfg.StartableStates()
}

func (e WorkflowEvaluator) TransitionTargetsWithRole(from State, role string) []string {
	return e.cfg.TransitionTargetsWithRole(string(from), role)
}

// DependencyBlocks reports whether a dependency should still block work.
// Missing dependencies are treated as blocking until healed.
func (e WorkflowEvaluator) DependencyBlocks(blocker *Ticket) bool {
	if blocker == nil {
		return true
	}
	return e.cfg.BlocksDependents(blocker.State)
}

