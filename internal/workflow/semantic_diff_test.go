package workflow

import (
	"strings"
	"testing"
)

func TestDiffWorkflowPolicyAndRenderHuman(t *testing.T) {
	before := []byte(`{
		"states": {
			"todo": ["in-progress"],
			"in-progress": ["in-review"],
			"in-review": ["done"]
		},
		"semantics": {
			"review": ["in-review"],
			"closure": ["done"],
			"human_only_closure": true
		}
	}`)
	after := []byte(`{
		"states": {
			"todo": ["in-review"],
			"in-progress": ["in-review"],
			"in-review": ["done", "archived"],
			"archived": []
		},
		"semantics": {
			"review": ["in-review", "qa"],
			"closure": ["done", "archived"],
			"human_only_closure": true
		}
	}`)

	diff, err := DiffWorkflowPolicy(before, after)
	if err != nil {
		t.Fatalf("DiffWorkflowPolicy failed: %v", err)
	}
	rendered := RenderWorkflowPolicyDiffHuman(diff)

	for _, needle := range []string{
		"Added states: archived",
		"todo transitions: +in-review -in-progress",
		"in-review transitions: +archived",
		"semantics.review changed",
		"semantics.closure changed",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("expected semantic diff output to include %q, got:\n%s", needle, rendered)
		}
	}
}
