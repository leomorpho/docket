package cmd

import "strings"

type startAgentQuickstart struct {
	DirectEditAvoidance string   `json:"direct_edit_avoidance"`
	CoreWorkflow        []string `json:"core_workflow"`
	CapabilityDiscovery []string `json:"capability_discovery"`
}

func buildStartAgentQuickstart() startAgentQuickstart {
	// Intentionally repeated every start run: reminder fatigue is preferable to
	// missed workflow guardrails when agents resume mid-stream or skip onboarding docs.
	return startAgentQuickstart{
		DirectEditAvoidance: "Never edit .docket/tickets/*.md directly; use `docket` commands so ticket signatures and metadata remain valid.",
		CoreWorkflow: []string{
			"docket list --state open --format context",
			"docket show TKT-NNN --format context",
			"docket update TKT-NNN --state in-progress",
			"docket ac check TKT-NNN",
		},
		CapabilityDiscovery: []string{
			"docket capabilities --format json",
			"docket doctor --format json",
			"docket help-json",
		},
	}
}

func renderStartAgentQuickstartHuman(q startAgentQuickstart) string {
	lines := []string{
		"Agent quickstart:",
		"- " + q.DirectEditAvoidance,
		"- Core workflow: " + strings.Join(q.CoreWorkflow, " | "),
		"- Capability discovery: " + strings.Join(q.CapabilityDiscovery, " | "),
	}
	return strings.Join(lines, "\n")
}
