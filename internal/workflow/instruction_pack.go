package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	InstructionPackVersion = 1
	DefaultInstructionPack = ".docket/instruction-pack.json"
)

type InstructionPack struct {
	Version          int                 `json:"version"`
	GeneratedAt      string              `json:"generated_at"`
	WorkflowLockHash string              `json:"workflow_lock_hash"`
	PromptPack       string              `json:"prompt_pack,omitempty"`
	States           map[string][]string `json:"states"`
	Routing          map[string]string   `json:"routing,omitempty"`
}

type instructionPolicyView struct {
	States     map[string][]string `json:"states"`
	PromptPack string              `json:"prompt_pack,omitempty"`
	Routing    map[string]string   `json:"routing,omitempty"`
}

func GenerateInstructionPack(lock WorkflowLock, workflowLockHash string) (InstructionPack, error) {
	var policy instructionPolicyView
	if err := json.Unmarshal(lock.Policy, &policy); err != nil {
		return InstructionPack{}, fmt.Errorf("%w: policy cannot be parsed for instruction pack", ErrWorkflowLockMalformed)
	}
	if len(policy.States) == 0 {
		return InstructionPack{}, fmt.Errorf("%w: policy states are required for instruction pack", ErrWorkflowLockMalformed)
	}
	return InstructionPack{
		Version:          InstructionPackVersion,
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		WorkflowLockHash: workflowLockHash,
		PromptPack:       policy.PromptPack,
		States:           policy.States,
		Routing:          policy.Routing,
	}, nil
}

func WriteInstructionPack(repoRoot, relPath string, pack InstructionPack) (string, error) {
	if relPath == "" {
		relPath = DefaultInstructionPack
	}
	path := relPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, relPath)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
