package workflow

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	WorkflowLockVersion   = 1
	SignatureAlgorithmV1  = "ed25519"
	DefaultWorkflowPolicy = ".docket/workflow.proposal.json"
	DefaultWorkflowLock   = ".docket/workflow.lock.json"
)

var (
	ErrWorkflowLockMalformed = errors.New("workflow lock is malformed")
	ErrWorkflowLockStale     = errors.New("workflow lock is stale relative to proposal")
	ErrWorkflowLockSignature = errors.New("workflow lock signature validation failed")
)

type SignatureMetadata struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type WorkflowLock struct {
	Version       int               `json:"version"`
	PolicyVersion int               `json:"policy_version"`
	ProposalPath  string            `json:"proposal_path"`
	ProposalHash  string            `json:"proposal_hash"`
	GeneratedAt   string            `json:"generated_at"`
	SignerID      string            `json:"signer_id"`
	SignerPublic  string            `json:"signer_public"`
	VersionHash   string            `json:"version_hash"`
	Signature     SignatureMetadata `json:"signature"`
	Policy        json.RawMessage   `json:"policy"`
}

type WorkflowSigner interface {
	DevicePublicKey() (ed25519.PublicKey, error)
	SignDevice(message []byte) ([]byte, error)
}

func GenerateWorkflowLock(repoRoot, proposalPath, signerID string, signer WorkflowSigner) (WorkflowLock, error) {
	if signerID == "" {
		return WorkflowLock{}, fmt.Errorf("signer_id is required")
	}
	if proposalPath == "" {
		proposalPath = DefaultWorkflowPolicy
	}
	proposalAbs := proposalPath
	if !filepath.IsAbs(proposalAbs) {
		proposalAbs = filepath.Join(repoRoot, proposalPath)
	}
	proposalBytes, err := os.ReadFile(proposalAbs)
	if err != nil {
		return WorkflowLock{}, err
	}
	var proposalPayload json.RawMessage
	if err := json.Unmarshal(proposalBytes, &proposalPayload); err != nil {
		return WorkflowLock{}, fmt.Errorf("%w: invalid proposal JSON", ErrWorkflowLockMalformed)
	}
	canonicalProposal, err := canonicalJSON(proposalPayload)
	if err != nil {
		return WorkflowLock{}, err
	}
	proposalHash := sha256.Sum256(canonicalProposal)

	pub, err := signer.DevicePublicKey()
	if err != nil {
		return WorkflowLock{}, err
	}
	lock := WorkflowLock{
		Version:       WorkflowLockVersion,
		PolicyVersion: 1,
		ProposalPath:  filepath.ToSlash(filepath.Clean(proposalPath)),
		ProposalHash:  hex.EncodeToString(proposalHash[:]),
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		SignerID:      signerID,
		SignerPublic:  base64.StdEncoding.EncodeToString(pub),
		Policy:        canonicalProposal,
	}
	lock.VersionHash = computeVersionHash(lock)

	signingPayload, err := signingPayload(lock)
	if err != nil {
		return WorkflowLock{}, err
	}
	sig, err := signer.SignDevice(signingPayload)
	if err != nil {
		return WorkflowLock{}, err
	}
	lock.Signature = SignatureMetadata{
		Algorithm: SignatureAlgorithmV1,
		Value:     base64.StdEncoding.EncodeToString(sig),
	}
	return lock, nil
}

func WriteWorkflowLock(repoRoot string, lock WorkflowLock) error {
	path := filepath.Join(repoRoot, DefaultWorkflowLock)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func ParseWorkflowLock(repoRoot string) (WorkflowLock, error) {
	path := filepath.Join(repoRoot, DefaultWorkflowLock)
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowLock{}, err
	}
	var lock WorkflowLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return WorkflowLock{}, fmt.Errorf("%w: invalid JSON", ErrWorkflowLockMalformed)
	}
	return lock, nil
}

func ValidateWorkflowLock(repoRoot string, lock WorkflowLock) error {
	if lock.Version != WorkflowLockVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrWorkflowLockMalformed, lock.Version)
	}
	if lock.SignerID == "" || lock.SignerPublic == "" {
		return fmt.Errorf("%w: signer metadata required", ErrWorkflowLockMalformed)
	}
	if lock.Signature.Algorithm != SignatureAlgorithmV1 || lock.Signature.Value == "" {
		return fmt.Errorf("%w: signature metadata invalid", ErrWorkflowLockMalformed)
	}
	if lock.VersionHash != computeVersionHash(lock) {
		return fmt.Errorf("%w: version hash mismatch", ErrWorkflowLockMalformed)
	}

	proposalAbs := filepath.Join(repoRoot, lock.ProposalPath)
	proposalBytes, err := os.ReadFile(proposalAbs)
	if err != nil {
		return err
	}
	var proposalPayload json.RawMessage
	if err := json.Unmarshal(proposalBytes, &proposalPayload); err != nil {
		return fmt.Errorf("%w: invalid proposal JSON", ErrWorkflowLockMalformed)
	}
	canonicalProposal, err := canonicalJSON(proposalPayload)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(canonicalProposal)
	if lock.ProposalHash != hex.EncodeToString(hash[:]) {
		return ErrWorkflowLockStale
	}

	pubRaw, err := base64.StdEncoding.DecodeString(lock.SignerPublic)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: signer public key invalid", ErrWorkflowLockMalformed)
	}
	sigRaw, err := base64.StdEncoding.DecodeString(lock.Signature.Value)
	if err != nil {
		return fmt.Errorf("%w: signature value invalid", ErrWorkflowLockMalformed)
	}

	payload, err := signingPayload(lock)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), payload, sigRaw) {
		return ErrWorkflowLockSignature
	}
	return nil
}

func signingPayload(lock WorkflowLock) ([]byte, error) {
	unsigned := struct {
		Version       int             `json:"version"`
		PolicyVersion int             `json:"policy_version"`
		ProposalPath  string          `json:"proposal_path"`
		ProposalHash  string          `json:"proposal_hash"`
		GeneratedAt   string          `json:"generated_at"`
		SignerID      string          `json:"signer_id"`
		SignerPublic  string          `json:"signer_public"`
		VersionHash   string          `json:"version_hash"`
		Policy        json.RawMessage `json:"policy"`
	}{
		Version:       lock.Version,
		PolicyVersion: lock.PolicyVersion,
		ProposalPath:  lock.ProposalPath,
		ProposalHash:  lock.ProposalHash,
		GeneratedAt:   lock.GeneratedAt,
		SignerID:      lock.SignerID,
		SignerPublic:  lock.SignerPublic,
		VersionHash:   lock.VersionHash,
		Policy:        lock.Policy,
	}
	return canonicalJSON(unsigned)
}

func computeVersionHash(lock WorkflowLock) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("v%d|p%d|%s|%s", lock.Version, lock.PolicyVersion, lock.ProposalPath, lock.ProposalHash)))
	return hex.EncodeToString(sum[:])
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
