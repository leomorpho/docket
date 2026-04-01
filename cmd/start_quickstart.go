package cmd

import (
	"sort"
	"strings"

	"github.com/leomorpho/docket/internal/ticket"
)

type startQuickstartSkill struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Intent  string `json:"intent"`
	Summary string `json:"summary"`
}

type startAgentQuickstart struct {
	DirectEditAvoidance string                 `json:"direct_edit_avoidance"`
	ManagedRunBinding   string                 `json:"managed_run_binding,omitempty"`
	SkillsReminder      string                 `json:"skills_reminder"`
	Skills              []startQuickstartSkill `json:"skills"`
	CoreWorkflow        []string               `json:"core_workflow"`
	CapabilityDiscovery []string               `json:"capability_discovery"`
}

func buildStartAgentQuickstart(repoRoot, managedBranch, managedWorktree string) startAgentQuickstart {
	// Defaults are only fallback examples when config cannot be loaded.
	workState := "running"
	finishState := "validated"
	if cfg, err := ticket.LoadConfig(repoRoot); err == nil {
		workState = preferredStateForRole(cfg, "active", workState)
		finishState = completedWorkflowState(cfg)
	}
	// Intentionally repeated every start run: reminder fatigue is preferable to
	// missed workflow guardrails when agents resume mid-stream or skip onboarding docs.
	out := startAgentQuickstart{
		DirectEditAvoidance: "Never edit .docket/tickets/*.md directly; use `docket` commands so ticket signatures and metadata remain valid.",
		SkillsReminder:      "Docket has built-in skills. Check `docket skill list --format json`, use `docket skill invoke <skill-id>` when a skill matches the task, and confirm usage with `docket skill audit`.",
		CoreWorkflow: []string{
			"docket list --state open --format context",
			"docket show TKT-NNN --format context",
			"docket update TKT-NNN --state " + workState,
			"docket ac check TKT-NNN",
		},
		CapabilityDiscovery: []string{
			"docket skill list --format json",
			"docket capabilities --format json",
			"docket doctor --format json",
			"docket help-json",
		},
	}
	if strings.TrimSpace(managedBranch) != "" && strings.TrimSpace(managedWorktree) != "" {
		out.ManagedRunBinding = "Stay on branch `" + managedBranch + "` and do the work in `" + managedWorktree + "`. If a ticket commit lands elsewhere, repair the managed branch before moving to `" + finishState + "`."
	}
	if payload, err := loadSkillListPayload(repoRoot); err == nil {
		out.Skills = make([]startQuickstartSkill, 0, len(payload.Skills))
		for _, skill := range payload.Skills {
			out.Skills = append(out.Skills, startQuickstartSkill{
				ID:      skill.ID,
				Title:   skill.Title,
				Intent:  skill.Intent,
				Summary: skill.Summary,
			})
		}
	}
	return out
}

func renderStartAgentQuickstartHuman(q startAgentQuickstart) string {
	lines := []string{
		"Agent quickstart:",
		"- " + q.DirectEditAvoidance,
	}
	if q.ManagedRunBinding != "" {
		lines = append(lines, "- Binding: "+q.ManagedRunBinding)
	}
	lines = append(lines, "- Skills: "+renderQuickstartSkillsLine(q))
	lines = append(lines,
		"- Core workflow: "+strings.Join(q.CoreWorkflow, " | "),
		"- Capability discovery: "+strings.Join(q.CapabilityDiscovery, " | "),
	)
	return strings.Join(lines, "\n")
}

func renderQuickstartSkillsLine(q startAgentQuickstart) string {
	line := "use `docket skill invoke <skill-id>` when one matches the task"
	grouped := groupQuickstartSkillsByIntent(q.Skills)
	if len(grouped) == 0 {
		return line + "; discover with `docket skill list --format json`; confirm usage with `docket skill audit`."
	}

	parts := make([]string, 0, len(grouped))
	intents := make([]string, 0, len(grouped))
	for intent := range grouped {
		intents = append(intents, intent)
	}
	sort.Strings(intents)
	for _, intent := range intents {
		parts = append(parts, intent+"="+strings.Join(grouped[intent], ","))
	}
	return line + "; built-ins by intent: " + strings.Join(parts, " | ") + "; confirm usage with `docket skill audit`."
}

func groupQuickstartSkillsByIntent(skills []startQuickstartSkill) map[string][]string {
	grouped := map[string][]string{}
	for _, skill := range skills {
		intent := strings.TrimSpace(skill.Intent)
		if intent == "" {
			intent = "other"
		}
		grouped[intent] = append(grouped[intent], skill.ID)
	}
	for intent := range grouped {
		sort.Strings(grouped[intent])
	}
	return grouped
}
