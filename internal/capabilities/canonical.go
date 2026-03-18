package capabilities

import "github.com/leomorpho/docket/internal/artifacts"

func CanonicalContractV1() Contract {
	return Contract{
		Version: ContractVersion,
		Workflow: WorkflowCapabilities{
			Phases: []string{"bootstrap", "start", "plan", "implement", "verify"},
		},
		Hooks: HookCapabilities{
			Namespace:  HookNamespaceSystem,
			Invocation: HookInvocationSystem,
			Execution:  HookExecutionInternal,
			Events: []HookEvent{
				{Name: "run.start", Mode: HookModeEnforcement},
				{Name: "ticket.review", Mode: HookModeEnforcement},
				{Name: "ticket.qa", Mode: HookModeAdvisory},
				{Name: "ticket.privileged", Mode: HookModeEnforcement},
			},
		},
		Skills: SkillInventory{
			Namespace:  SkillNamespaceAgent,
			Invocation: SkillInvocationAgent,
			Inventory: []Skill{
				{
					Name:     "ticket-discovery",
					Title:    "Discover Next Ticket",
					Summary:  "Find the next actionable ticket and inspect its working context before coding.",
					Intent:   "planning",
					Command:  "docket list --state open --format context",
					Triggers: []string{"session_start", "resume", "task_selection"},
				},
				{
					Name:     "ticket-authoring-apply",
					Title:    "Transactional Ticket Authoring",
					Summary:  "Use scaffold/apply commands to author or update ticket specs without fragile shell quoting.",
					Intent:   "authoring",
					Command:  "docket ticket scaffold --format json",
					Triggers: []string{"multi_line_ticket_edit", "bulk_ticket_changes", "automation_mode"},
				},
				{
					Name:     "context-optimize",
					Title:    "Compact Ticket Brief",
					Summary:  "Generate a bounded brief from ticket context, learnings, and recent activity.",
					Intent:   "context",
					Command:  "docket context optimize {ticket_id}",
					Triggers: []string{"llm_context_budget", "ticket_handoff", "task_brief"},
					Optional: true,
				},
				{
					Name:     "learning-replay",
					Title:    "Replay Relevant Learnings",
					Summary:  "Replay top ranked learned rules for a ticket using the same ranking model as start.",
					Intent:   "quality",
					Command:  "docket learn replay {ticket_id}",
					Triggers: []string{"pre_implementation", "incident_recurrence", "ticket_resume"},
					Optional: true,
				},
				{
					Name:     "wrap-up-readiness",
					Title:    "End-of-Session Wrap-Up",
					Summary:  "Run wrap-up readiness checks for AC completion, handoff quality, blockers, and review transition readiness.",
					Intent:   "review",
					Command:  "docket wrap-up {ticket_id}",
					Triggers: []string{"session_end", "pre_review", "handoff"},
					Optional: true,
				},
			},
		},
		Compatibility: CompatibilityNotes{
			BackwardCompatibleWith: []int{ContractVersion},
			UpgradeNotes:           "Regenerate adapter skill packs when the canonical contract hash changes.",
		},
	}
}

func EnsureRuntimeContract(repoRoot string) (RuntimeContract, string, error) {
	runtime, err := LoadRuntimeContract(repoRoot)
	if err == nil {
		return runtime, artifacts.WriteRepoPath(repoRoot, artifacts.RepoRuntimeCapabilities), nil
	}
	return WriteRuntimeContract(repoRoot, CanonicalContractV1())
}
