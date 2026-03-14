package workflow

import (
	"encoding/json"
	"testing"
)

func TestGenerateInstructionPackFromWorkflowLock(t *testing.T) {
	lock := WorkflowLock{
		Policy: []byte(`{"states":{"todo":["in-progress"],"in-progress":["in-review"]},"prompt_pack":"base-v1","routing":{"small":"cheap","large":"strong"}}`),
	}
	pack, err := GenerateInstructionPack(lock, "hash-123")
	if err != nil {
		t.Fatalf("GenerateInstructionPack failed: %v", err)
	}
	if pack.WorkflowLockHash != "hash-123" {
		t.Fatalf("expected workflow lock hash to be pinned, got %q", pack.WorkflowLockHash)
	}
	if pack.PromptPack != "base-v1" {
		t.Fatalf("unexpected prompt pack: %q", pack.PromptPack)
	}
	if len(pack.States) == 0 || len(pack.Routing) == 0 {
		t.Fatalf("expected states and routing in pack: %#v", pack)
	}
}

func TestGenerateInstructionPackChangesWhenPolicyChanges(t *testing.T) {
	lockA := WorkflowLock{
		Policy: []byte(`{"states":{"todo":["in-progress"]},"prompt_pack":"base-v1","routing":{"small":"cheap"}}`),
	}
	lockB := WorkflowLock{
		Policy: []byte(`{"states":{"todo":["in-progress"]},"prompt_pack":"base-v2","routing":{"small":"balanced"}}`),
	}
	packA, err := GenerateInstructionPack(lockA, "hash-a")
	if err != nil {
		t.Fatalf("GenerateInstructionPack(lockA) failed: %v", err)
	}
	packB, err := GenerateInstructionPack(lockB, "hash-b")
	if err != nil {
		t.Fatalf("GenerateInstructionPack(lockB) failed: %v", err)
	}
	a, _ := json.Marshal(packA)
	b, _ := json.Marshal(packB)
	if string(a) == string(b) {
		t.Fatalf("expected instruction pack output to change when policy changes")
	}
}
