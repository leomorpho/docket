package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestBuildStartAgentQuickstartContentAndConciseness(t *testing.T) {
	h := newFakeRepoHarness(t)
	quickstart := buildStartAgentQuickstart(h.repo, "docket/TKT-999", "/tmp/docket/TKT-999")

	if !strings.Contains(strings.ToLower(quickstart.DirectEditAvoidance), "never edit .docket/tickets") {
		t.Fatalf("direct edit guidance missing ticket file guardrail: %q", quickstart.DirectEditAvoidance)
	}
	if !strings.Contains(strings.Join(quickstart.CoreWorkflow, "\n"), "docket list --state open --format context") {
		t.Fatalf("core workflow missing list context command: %#v", quickstart.CoreWorkflow)
	}
	if !strings.Contains(strings.Join(quickstart.CoreWorkflow, "\n"), "docket show TKT-NNN --format context") {
		t.Fatalf("core workflow missing show context command: %#v", quickstart.CoreWorkflow)
	}
	if !strings.Contains(strings.Join(quickstart.CoreWorkflow, "\n"), "docket update TKT-NNN --state in-progress") {
		t.Fatalf("core workflow missing update command: %#v", quickstart.CoreWorkflow)
	}
	if !strings.Contains(strings.Join(quickstart.CapabilityDiscovery, "\n"), "docket capabilities") {
		t.Fatalf("capability discovery missing capabilities entrypoint: %#v", quickstart.CapabilityDiscovery)
	}
	if !strings.Contains(quickstart.SkillsReminder, "docket skill list --format json") {
		t.Fatalf("skills reminder missing skill list guidance: %#v", quickstart)
	}
	if !strings.Contains(quickstart.SkillsReminder, "docket skill audit") {
		t.Fatalf("skills reminder missing audit guidance: %#v", quickstart)
	}
	if !strings.Contains(quickstart.ManagedRunBinding, "docket/TKT-999") {
		t.Fatalf("managed binding missing branch reminder: %#v", quickstart)
	}
	if len(quickstart.Skills) == 0 {
		t.Fatalf("expected built-in skills in quickstart, got %#v", quickstart)
	}
	if quickstart.Skills[0].ID == "" || quickstart.Skills[0].Title == "" {
		t.Fatalf("expected skill ids and titles in quickstart, got %#v", quickstart.Skills)
	}

	rendered := renderStartAgentQuickstartHuman(quickstart)
	if !strings.Contains(rendered, "Agent quickstart:") {
		t.Fatalf("human render missing quickstart heading: %q", rendered)
	}
	if !strings.Contains(rendered, "Binding:") {
		t.Fatalf("human render missing binding guidance: %q", rendered)
	}
	if !strings.Contains(rendered, "built-ins by intent:") {
		t.Fatalf("human render missing grouped skills summary: %q", rendered)
	}
	if !strings.Contains(rendered, "planning=ticket-discovery") {
		t.Fatalf("human render missing planning skill grouping: %q", rendered)
	}
	if len(rendered) > 1200 {
		t.Fatalf("quickstart should stay compact (<700 chars), got %d chars\n%s", len(rendered), rendered)
	}
}

func TestBuildStartAgentQuickstartUsesConfiguredActiveStateName(t *testing.T) {
	h := newFakeRepoHarness(t)
	cfg := &ticket.Config{
		Backend: "local",
		States: map[string]ticket.StateConfig{
			"queued": {
				Label:            "Queued",
				Open:             true,
				Column:           0,
				Next:             []string{"building"},
				Roles:            []string{"intake"},
				Startable:        true,
				BlocksDependents: true,
			},
			"building": {
				Label:            "Building",
				Open:             true,
				Column:           1,
				Next:             []string{"qa"},
				Roles:            []string{"active"},
				BlocksDependents: true,
			},
			"qa": {
				Label:            "QA",
				Open:             true,
				Column:           2,
				Next:             []string{"shipped"},
				Roles:            []string{"review"},
				Reviewable:       true,
				BlocksDependents: true,
			},
			"shipped": {
				Label:    "Shipped",
				Open:     false,
				Column:   3,
				Next:     []string{},
				Roles:    []string{"completed"},
				Terminal: true,
			},
		},
		DefaultState:    "queued",
		DefaultPriority: 10,
		HandoffSections: ticket.DefaultConfig().HandoffSections,
	}
	if err := ticket.SaveConfig(h.repo, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	quickstart := buildStartAgentQuickstart(h.repo, "docket/TKT-999", "/tmp/docket/TKT-999")
	if !strings.Contains(strings.Join(quickstart.CoreWorkflow, "\n"), "docket update TKT-NNN --state building") {
		t.Fatalf("expected quickstart to use configured active state, got %#v", quickstart.CoreWorkflow)
	}
}

func TestStartOutputIncludesAgentQuickstartForHumanAndJSON(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-970", 970, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})
	h.seedTicket("TKT-971", 971, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	humanOut, err := h.run("start")
	if err != nil {
		t.Fatalf("start human failed: %v\n%s", err, humanOut)
	}
	if !strings.Contains(humanOut, "Agent quickstart:") {
		t.Fatalf("expected human start output quickstart block, got:\n%s", humanOut)
	}
	if !strings.Contains(strings.ToLower(humanOut), "never edit .docket/tickets") {
		t.Fatalf("expected direct-edit avoidance guidance in human output, got:\n%s", humanOut)
	}
	if !strings.Contains(humanOut, "docket capabilities --format json") {
		t.Fatalf("expected capability discovery command in human output, got:\n%s", humanOut)
	}
	if !strings.Contains(humanOut, "docket skill list --format json") {
		t.Fatalf("expected skill list reminder in human output, got:\n%s", humanOut)
	}
	if !strings.Contains(humanOut, "docket skill invoke <skill-id>") {
		t.Fatalf("expected skill invoke reminder in human output, got:\n%s", humanOut)
	}
	if !strings.Contains(humanOut, "Managed run binding: branch=docket/TKT-970") {
		t.Fatalf("expected managed run binding in human output, got:\n%s", humanOut)
	}
	if !strings.Contains(humanOut, "quality=learning-replay") {
		t.Fatalf("expected grouped concrete skill inventory in human output, got:\n%s", humanOut)
	}

	jsonOut, err := h.run("start", "--format", "json")
	if err != nil {
		t.Fatalf("start json failed: %v\n%s", err, jsonOut)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal start json failed: %v\n%s", err, jsonOut)
	}
	quickRaw, ok := payload["agent_quickstart"]
	if !ok {
		t.Fatalf("expected agent_quickstart in start json payload, got %#v", payload)
	}
	quickJSON, err := json.Marshal(quickRaw)
	if err != nil {
		t.Fatalf("marshal agent_quickstart failed: %v", err)
	}
	var quickstart startAgentQuickstart
	if err := json.Unmarshal(quickJSON, &quickstart); err != nil {
		t.Fatalf("decode agent_quickstart failed: %v (%s)", err, string(quickJSON))
	}

	if !strings.Contains(strings.ToLower(quickstart.DirectEditAvoidance), "never edit .docket/tickets") {
		t.Fatalf("json quickstart missing direct-edit guidance: %#v", quickstart)
	}
	if !strings.Contains(quickstart.SkillsReminder, "docket skill list --format json") {
		t.Fatalf("json quickstart missing skills reminder: %#v", quickstart)
	}
	if !strings.Contains(quickstart.ManagedRunBinding, "docket/TKT-971") {
		t.Fatalf("json quickstart missing binding guidance: %#v", quickstart)
	}
	if len(quickstart.Skills) == 0 {
		t.Fatalf("json quickstart missing concrete skills inventory: %#v", quickstart)
	}
	if len(strings.Join(quickstart.CoreWorkflow, "\n"))+len(strings.Join(quickstart.CapabilityDiscovery, "\n")) > 900 {
		t.Fatalf("json quickstart should remain compact, got %#v", quickstart)
	}
}
