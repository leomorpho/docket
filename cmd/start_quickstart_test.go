package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestBuildStartAgentQuickstartContentAndConciseness(t *testing.T) {
	quickstart := buildStartAgentQuickstart()

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

	rendered := renderStartAgentQuickstartHuman(quickstart)
	if !strings.Contains(rendered, "Agent quickstart:") {
		t.Fatalf("human render missing quickstart heading: %q", rendered)
	}
	if len(rendered) > 700 {
		t.Fatalf("quickstart should stay compact (<700 chars), got %d chars\n%s", len(rendered), rendered)
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
	if len(strings.Join(quickstart.CoreWorkflow, "\n"))+len(strings.Join(quickstart.CapabilityDiscovery, "\n")) > 900 {
		t.Fatalf("json quickstart should remain compact, got %#v", quickstart)
	}
}
