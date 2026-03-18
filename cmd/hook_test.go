package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leomorpho/docket/internal/capabilities"
)

func TestHookListShowAndStatusAreIntrospectionOnly(t *testing.T) {
	h := newFakeRepoHarness(t)

	listOut, err := h.run("hook", "list", "--format", "json")
	if err != nil {
		t.Fatalf("hook list failed: %v\n%s", err, listOut)
	}
	var list map[string]any
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("unmarshal hook list json failed: %v\n%s", err, listOut)
	}
	events, ok := list["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("expected hook events in list output, got %#v", list["events"])
	}
	name := events[0].(map[string]any)["name"].(string)

	showOut, err := h.run("hook", "show", name, "--format", "json")
	if err != nil {
		t.Fatalf("hook show failed: %v\n%s", err, showOut)
	}
	var show map[string]any
	if err := json.Unmarshal([]byte(showOut), &show); err != nil {
		t.Fatalf("unmarshal hook show json failed: %v\n%s", err, showOut)
	}
	event := show["event"].(map[string]any)
	if event["name"] != name {
		t.Fatalf("expected hook show event %s, got %#v", name, event)
	}
	if show["execution"] != capabilities.HookExecutionInternal {
		t.Fatalf("expected internal-only execution mode, got %#v", show["execution"])
	}

	runOut, err := h.run("hook", "run", name)
	if err != nil {
		t.Fatalf("hook run introspection fallback should not hard-fail, got err=%v output=%s", err, runOut)
	}
	if strings.Contains(runOut, "run        ") || strings.Contains(runOut, " run ") {
		t.Fatalf("hook command should not expose executable run surface, got:\n%s", runOut)
	}
	if !strings.Contains(runOut, "Available Commands:") {
		t.Fatalf("expected hook help output for unavailable subcommand, got:\n%s", runOut)
	}
}

func TestHookStatusReportsDegradedAndReadyStates(t *testing.T) {
	h := newFakeRepoHarness(t)

	beforeOut, err := h.run("hook", "status", "--format", "json")
	if err != nil {
		t.Fatalf("hook status before bootstrap failed: %v\n%s", err, beforeOut)
	}
	var before map[string]any
	if err := json.Unmarshal([]byte(beforeOut), &before); err != nil {
		t.Fatalf("unmarshal hook status before json failed: %v\n%s", err, beforeOut)
	}
	if before["ready"] != false {
		t.Fatalf("expected hook readiness false before bootstrap, got %#v", before["ready"])
	}

	if out, err := h.run("bootstrap", "--adapter", "codex"); err != nil {
		t.Fatalf("bootstrap failed: %v\n%s", err, out)
	}

	afterOut, err := h.run("hook", "status", "--format", "json")
	if err != nil {
		t.Fatalf("hook status after bootstrap failed: %v\n%s", err, afterOut)
	}
	var after map[string]any
	if err := json.Unmarshal([]byte(afterOut), &after); err != nil {
		t.Fatalf("unmarshal hook status after json failed: %v\n%s", err, afterOut)
	}
	if after["ready"] != true {
		t.Fatalf("expected hook readiness true after bootstrap, got %#v", after["ready"])
	}
	if after["invocation"] != capabilities.HookInvocationSystem {
		t.Fatalf("expected hook invocation %q, got %#v", capabilities.HookInvocationSystem, after["invocation"])
	}
}
