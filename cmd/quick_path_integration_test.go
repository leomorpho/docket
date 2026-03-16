package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestStartAndCapabilitiesPrioritizeApplyQuickPath(t *testing.T) {
	h := newFakeRepoHarness(t)
	h.seedTicket("TKT-980", 980, ticket.State("todo"), []ticket.AcceptanceCriterion{{Description: "ac"}})

	if out, err := h.run("bootstrap", "--adapter", "codex"); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}

	startOut, err := h.run("start", "--format", "json")
	if err != nil {
		t.Fatalf("start json failed: %v\n%s", err, startOut)
	}
	var startPayload map[string]any
	if err := json.Unmarshal([]byte(startOut), &startPayload); err != nil {
		t.Fatalf("unmarshal start payload failed: %v\n%s", err, startOut)
	}
	startQuick, ok := startPayload["llm_quick_path"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_quick_path in start output, got %#v", startPayload)
	}
	assertQuickPathFields(t, startQuick)

	capOut, err := h.run("capabilities", "--format", "json")
	if err != nil {
		t.Fatalf("capabilities json failed: %v\n%s", err, capOut)
	}
	var capPayload map[string]any
	if err := json.Unmarshal([]byte(capOut), &capPayload); err != nil {
		t.Fatalf("unmarshal capabilities payload failed: %v\n%s", err, capOut)
	}
	capQuick, ok := capPayload["llm_quick_path"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_quick_path in capabilities output, got %#v", capPayload)
	}
	assertQuickPathFields(t, capQuick)

	trace := strings.Join([]string{
		startQuick["ticket_apply"].(string),
		startQuick["backlog_apply"].(string),
		startQuick["proof_attach"].(string),
		startQuick["proof_verify"].(string),
		capQuick["ticket_apply"].(string),
		capQuick["backlog_apply"].(string),
		capQuick["proof_attach"].(string),
		capQuick["proof_verify"].(string),
	}, "\n")
	startFixture := h.writeFixture(filepath.Join("quick-path", "start.json"), []byte(startOut))
	capFixture := h.writeFixture(filepath.Join("quick-path", "capabilities.json"), []byte(capOut))
	traceFixture := h.writeFixture(filepath.Join("quick-path", "command-trace.txt"), []byte(trace+"\n"))
	t.Logf("quick-path fixtures: %s | %s | %s", startFixture, capFixture, traceFixture)
}

func assertQuickPathFields(t *testing.T, payload map[string]any) {
	t.Helper()
	for _, key := range []string{"preference", "ticket_apply", "backlog_apply", "proof_attach", "proof_verify", "automation_hint"} {
		if payload[key] == nil {
			t.Fatalf("quick path missing %s field: %#v", key, payload)
		}
	}
	if !strings.Contains(payload["ticket_apply"].(string), "ticket apply") || !strings.Contains(payload["ticket_apply"].(string), "--automation") {
		t.Fatalf("ticket quick path must prioritize transactional apply + automation: %#v", payload)
	}
	if !strings.Contains(payload["backlog_apply"].(string), "backlog apply") || !strings.Contains(payload["backlog_apply"].(string), "--automation") {
		t.Fatalf("backlog quick path must prioritize transactional apply + automation: %#v", payload)
	}
	if !strings.Contains(payload["proof_attach"].(string), "proof add") || !strings.Contains(payload["proof_attach"].(string), "--proof-title") || !strings.Contains(payload["proof_attach"].(string), "--note") {
		t.Fatalf("proof_attach quick path must include canonical proof add command with title+note: %#v", payload)
	}
	if !strings.Contains(payload["proof_verify"].(string), "proof list") || !strings.Contains(payload["proof_verify"].(string), "show") {
		t.Fatalf("proof_verify quick path must include list/show verification commands: %#v", payload)
	}
	if !strings.Contains(payload["proof_attach"].(string), "--format json") || !strings.Contains(payload["proof_verify"].(string), "--format json") {
		t.Fatalf("proof quick path should include machine-readable JSON hints: %#v", payload)
	}
	if !strings.Contains(strings.ToLower(payload["preference"].(string)), "transactional") {
		t.Fatalf("preference should emphasize transactional apply flow: %#v", payload)
	}
}
