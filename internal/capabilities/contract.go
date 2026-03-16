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
)

const (
	ContractVersion            = 1
	DefaultRuntimeContractPath = ".docket/runtime/capabilities.json"
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
	Events []HookEvent `json:"events"`
}

type HookEvent struct {
	Name     string `json:"name"`
	Blocking bool   `json:"blocking"`
}

type SkillInventory struct {
	Inventory []Skill `json:"inventory"`
}

type Skill struct {
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
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
	if len(normalized.Hooks.Events) == 0 {
		return fmt.Errorf("%w: hooks.events is required", ErrInvalidContract)
	}
	for i, event := range normalized.Hooks.Events {
		if strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("%w: hooks.events[%d].name is required", ErrInvalidContract, i)
		}
	}
	if len(normalized.Skills.Inventory) == 0 {
		return fmt.Errorf("%w: skills.inventory is required", ErrInvalidContract)
	}
	for i, skill := range normalized.Skills.Inventory {
		if strings.TrimSpace(skill.Name) == "" {
			return fmt.Errorf("%w: skills.inventory[%d].name is required", ErrInvalidContract, i)
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
	path := filepath.Join(repoRoot, DefaultRuntimeContractPath)
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
