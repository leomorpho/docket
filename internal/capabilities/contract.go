package capabilities

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leomorpho/docket/internal/artifacts"
)

const (
	ContractVersion            = 1
	DefaultRuntimeContractPath = ".docket/local/runtime/capabilities.json"

	SkillNamespaceAgent = "docket.skill"
	HookNamespaceSystem = "docket.hook"

	SkillInvocationAgent  = "agent-invoked"
	HookInvocationSystem  = "system-run"
	HookExecutionInternal = "internal-only"

	HookModeAdvisory    = "advisory"
	HookModeEnforcement = "enforcement"
)

var (
	ErrInvalidContract = errors.New("invalid capabilities contract")
	ErrHashMismatch    = errors.New("capabilities contract hash mismatch")
)

// Contract is the canonical capabilities payload used by adapters and runtime surfaces.
type Contract struct {
	Version       int                  `json:"version"`
	Workflow      WorkflowCapabilities `json:"workflow"`
	Hooks         HookCapabilities     `json:"hooks"`
	Skills        SkillInventory       `json:"skills"`
	Compatibility CompatibilityNotes   `json:"compatibility"`
}

type WorkflowCapabilities struct {
	Phases []string `json:"phases"`
}

type HookCapabilities struct {
	Namespace  string      `json:"namespace,omitempty"`
	Invocation string      `json:"invocation,omitempty"`
	Execution  string      `json:"execution,omitempty"`
	Events     []HookEvent `json:"events"`
}

type HookEvent struct {
	Name     string `json:"name"`
	Mode     string `json:"mode,omitempty"`
	Blocking bool   `json:"blocking,omitempty"`
}

type SkillInventory struct {
	Namespace  string  `json:"namespace,omitempty"`
	Invocation string  `json:"invocation,omitempty"`
	Inventory  []Skill `json:"inventory"`
}

type Skill struct {
	Name     string   `json:"name"`
	Title    string   `json:"title,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Intent   string   `json:"intent,omitempty"`
	Command  string   `json:"command,omitempty"`
	Triggers []string `json:"triggers,omitempty"`
	Optional bool     `json:"optional"`
}

// CompatibilityNotes documents versioning and upgrade constraints for future schema evolution.
type CompatibilityNotes struct {
	BackwardCompatibleWith []int  `json:"backward_compatible_with,omitempty"`
	UpgradeNotes           string `json:"upgrade_notes"`
}

type RuntimeContract struct {
	Version       int                  `json:"version"`
	Hash          string               `json:"hash"`
	Workflow      WorkflowCapabilities `json:"workflow"`
	Hooks         HookCapabilities     `json:"hooks"`
	Skills        SkillInventory       `json:"skills"`
	Compatibility CompatibilityNotes   `json:"compatibility"`
}

func ValidateContract(contract Contract) error {
	normalized, err := normalize(contract)
	if err != nil {
		return err
	}
	if len(normalized.Workflow.Phases) == 0 {
		return fmt.Errorf("%w: workflow.phases is required", ErrInvalidContract)
	}
	for i, phase := range normalized.Workflow.Phases {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("%w: workflow.phases[%d] cannot be empty", ErrInvalidContract, i)
		}
	}
	if normalized.Hooks.Namespace != HookNamespaceSystem {
		return fmt.Errorf("%w: hooks.namespace must be %q", ErrInvalidContract, HookNamespaceSystem)
	}
	if normalized.Hooks.Invocation != HookInvocationSystem {
		return fmt.Errorf("%w: hooks.invocation must be %q", ErrInvalidContract, HookInvocationSystem)
	}
	if normalized.Hooks.Execution != HookExecutionInternal {
		return fmt.Errorf("%w: hooks.execution must be %q", ErrInvalidContract, HookExecutionInternal)
	}
	if len(normalized.Hooks.Events) == 0 {
		return fmt.Errorf("%w: hooks.events is required", ErrInvalidContract)
	}
	for i, event := range normalized.Hooks.Events {
		if strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("%w: hooks.events[%d].name is required", ErrInvalidContract, i)
		}
		switch event.Mode {
		case HookModeAdvisory, HookModeEnforcement:
		default:
			return fmt.Errorf("%w: hooks.events[%d].mode must be %q or %q", ErrInvalidContract, i, HookModeAdvisory, HookModeEnforcement)
		}
	}
	if normalized.Skills.Namespace != SkillNamespaceAgent {
		return fmt.Errorf("%w: skills.namespace must be %q", ErrInvalidContract, SkillNamespaceAgent)
	}
	if normalized.Skills.Invocation != SkillInvocationAgent {
		return fmt.Errorf("%w: skills.invocation must be %q", ErrInvalidContract, SkillInvocationAgent)
	}
	if len(normalized.Skills.Inventory) == 0 {
		return fmt.Errorf("%w: skills.inventory is required", ErrInvalidContract)
	}
	for i, skill := range normalized.Skills.Inventory {
		if strings.TrimSpace(skill.Name) == "" {
			return fmt.Errorf("%w: skills.inventory[%d].name is required", ErrInvalidContract, i)
		}
		if strings.TrimSpace(skill.Title) == "" {
			return fmt.Errorf("%w: skills.inventory[%d].title is required", ErrInvalidContract, i)
		}
		if strings.TrimSpace(skill.Summary) == "" {
			return fmt.Errorf("%w: skills.inventory[%d].summary is required", ErrInvalidContract, i)
		}
		if strings.TrimSpace(skill.Intent) == "" {
			return fmt.Errorf("%w: skills.inventory[%d].intent is required", ErrInvalidContract, i)
		}
		if strings.TrimSpace(skill.Command) == "" {
			return fmt.Errorf("%w: skills.inventory[%d].command is required", ErrInvalidContract, i)
		}
		if len(skill.Triggers) == 0 {
			return fmt.Errorf("%w: skills.inventory[%d].triggers is required", ErrInvalidContract, i)
		}
	}
	if strings.TrimSpace(normalized.Compatibility.UpgradeNotes) == "" {
		return fmt.Errorf("%w: compatibility.upgrade_notes is required", ErrInvalidContract)
	}
	return nil
}

func HashContract(contract Contract) (string, error) {
	normalized, err := normalize(contract)
	if err != nil {
		return "", err
	}
	if err := ValidateContract(normalized); err != nil {
		return "", err
	}
	b, err := canonicalJSON(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func WriteRuntimeContract(repoRoot string, contract Contract) (RuntimeContract, string, error) {
	normalized, err := normalize(contract)
	if err != nil {
		return RuntimeContract{}, "", err
	}
	if err := ValidateContract(normalized); err != nil {
		return RuntimeContract{}, "", err
	}
	hash, err := HashContract(normalized)
	if err != nil {
		return RuntimeContract{}, "", err
	}
	runtime := RuntimeContract{
		Version:       normalized.Version,
		Hash:          hash,
		Workflow:      normalized.Workflow,
		Hooks:         normalized.Hooks,
		Skills:        normalized.Skills,
		Compatibility: normalized.Compatibility,
	}

	path := filepath.Join(repoRoot, DefaultRuntimeContractPath)
	path = artifacts.WriteRepoPath(repoRoot, artifacts.RepoRuntimeCapabilities)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return RuntimeContract{}, "", err
	}
	data, err := json.MarshalIndent(runtime, "", "  ")
	if err != nil {
		return RuntimeContract{}, "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return RuntimeContract{}, "", err
	}
	return runtime, path, nil
}

func LoadRuntimeContract(repoRoot string) (RuntimeContract, error) {
	path := artifacts.ReadRepoPath(repoRoot, artifacts.RepoRuntimeCapabilities)
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeContract{}, err
	}
	var runtime RuntimeContract
	if err := json.Unmarshal(data, &runtime); err != nil {
		return RuntimeContract{}, fmt.Errorf("%w: invalid runtime contract JSON", ErrInvalidContract)
	}
	contract := Contract{
		Version:       runtime.Version,
		Workflow:      runtime.Workflow,
		Hooks:         runtime.Hooks,
		Skills:        runtime.Skills,
		Compatibility: runtime.Compatibility,
	}
	if err := ValidateContract(contract); err != nil {
		return RuntimeContract{}, err
	}
	expectedHash, err := HashContract(contract)
	if err != nil {
		return RuntimeContract{}, err
	}
	if runtime.Hash != expectedHash {
		return RuntimeContract{}, fmt.Errorf("%w: expected %s got %s", ErrHashMismatch, expectedHash, runtime.Hash)
	}
	return runtime, nil
}

func normalize(contract Contract) (Contract, error) {
	c := contract
	if c.Version == 0 {
		c.Version = ContractVersion
	}
	if c.Version != ContractVersion {
		return Contract{}, fmt.Errorf("%w: unsupported version %d", ErrInvalidContract, c.Version)
	}
	if strings.TrimSpace(c.Hooks.Namespace) == "" {
		c.Hooks.Namespace = HookNamespaceSystem
	}
	if strings.TrimSpace(c.Hooks.Invocation) == "" {
		c.Hooks.Invocation = HookInvocationSystem
	}
	if strings.TrimSpace(c.Hooks.Execution) == "" {
		c.Hooks.Execution = HookExecutionInternal
	}
	for i := range c.Hooks.Events {
		mode := strings.TrimSpace(strings.ToLower(c.Hooks.Events[i].Mode))
		switch mode {
		case "":
			if c.Hooks.Events[i].Blocking {
				mode = HookModeEnforcement
			} else {
				mode = HookModeAdvisory
			}
		case HookModeAdvisory, HookModeEnforcement:
		default:
			return Contract{}, fmt.Errorf("%w: hooks.events[%d].mode must be %q or %q", ErrInvalidContract, i, HookModeAdvisory, HookModeEnforcement)
		}
		c.Hooks.Events[i].Mode = mode
		c.Hooks.Events[i].Blocking = mode == HookModeEnforcement
	}
	if strings.TrimSpace(c.Skills.Namespace) == "" {
		c.Skills.Namespace = SkillNamespaceAgent
	}
	if strings.TrimSpace(c.Skills.Invocation) == "" {
		c.Skills.Invocation = SkillInvocationAgent
	}
	for i := range c.Skills.Inventory {
		id := strings.TrimSpace(c.Skills.Inventory[i].Name)
		if strings.TrimSpace(c.Skills.Inventory[i].Title) == "" {
			c.Skills.Inventory[i].Title = id
		}
		if strings.TrimSpace(c.Skills.Inventory[i].Summary) == "" {
			c.Skills.Inventory[i].Summary = fmt.Sprintf("Use `%s` when the task matches this capability.", id)
		}
		if strings.TrimSpace(c.Skills.Inventory[i].Intent) == "" {
			c.Skills.Inventory[i].Intent = "on-demand"
		}
		if strings.TrimSpace(c.Skills.Inventory[i].Command) == "" {
			c.Skills.Inventory[i].Command = fmt.Sprintf("docket skill show %s", id)
		}
		seen := map[string]struct{}{}
		triggers := make([]string, 0, len(c.Skills.Inventory[i].Triggers))
		for _, raw := range c.Skills.Inventory[i].Triggers {
			item := strings.TrimSpace(raw)
			if item == "" {
				continue
			}
			key := strings.ToLower(item)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			triggers = append(triggers, item)
		}
		if len(triggers) == 0 {
			triggers = []string{"manual"}
		}
		c.Skills.Inventory[i].Triggers = triggers
	}
	return c, nil
}

func canonicalJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
