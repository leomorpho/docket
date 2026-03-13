package workflow

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testSigner struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func newTestSigner(t *testing.T) testSigner {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	return testSigner{pub: pub, priv: priv}
}

func (s testSigner) DevicePublicKey() (ed25519.PublicKey, error) { return s.pub, nil }
func (s testSigner) SignDevice(message []byte) ([]byte, error) {
	return ed25519.Sign(s.priv, message), nil
}

func TestGenerateParseValidateWorkflowLock(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	proposalPath := filepath.Join(repo, DefaultWorkflowPolicy)
	proposal := `{"states":{"todo":["in-progress"],"in-progress":["in-review"]},"prompt_pack":"base-v1","routing":{"small":"fast","large":"safe"}}`
	if err := os.WriteFile(proposalPath, []byte(proposal), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}

	signer := newTestSigner(t)
	lock, err := GenerateWorkflowLock(repo, DefaultWorkflowPolicy, "signer-local", signer)
	if err != nil {
		t.Fatalf("generate lock failed: %v", err)
	}
	if lock.Signature.Algorithm != SignatureAlgorithmV1 || lock.Signature.Value == "" {
		t.Fatalf("signature metadata missing: %+v", lock.Signature)
	}
	if err := WriteWorkflowLock(repo, lock); err != nil {
		t.Fatalf("write lock failed: %v", err)
	}

	parsed, err := ParseWorkflowLock(repo)
	if err != nil {
		t.Fatalf("parse lock failed: %v", err)
	}
	if err := ValidateWorkflowLock(repo, parsed); err != nil {
		t.Fatalf("validate lock failed: %v", err)
	}
}

func TestValidateWorkflowLockRejectsMalformedAndStale(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	proposalPath := filepath.Join(repo, DefaultWorkflowPolicy)
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["done"]}}`), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}
	signer := newTestSigner(t)
	lock, err := GenerateWorkflowLock(repo, DefaultWorkflowPolicy, "signer-local", signer)
	if err != nil {
		t.Fatalf("generate lock failed: %v", err)
	}

	lock.Signature.Algorithm = ""
	if err := ValidateWorkflowLock(repo, lock); !errors.Is(err, ErrWorkflowLockMalformed) {
		t.Fatalf("expected malformed lock error for bad signature metadata, got: %v", err)
	}

	lock, err = GenerateWorkflowLock(repo, DefaultWorkflowPolicy, "signer-local", signer)
	if err != nil {
		t.Fatalf("generate lock failed: %v", err)
	}
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["in-progress"]}}`), 0o644); err != nil {
		t.Fatalf("mutate proposal failed: %v", err)
	}
	if err := ValidateWorkflowLock(repo, lock); !errors.Is(err, ErrWorkflowLockStale) {
		t.Fatalf("expected stale lock error, got: %v", err)
	}
}

func TestValidateWorkflowLockRejectsInvalidSignature(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	proposalPath := filepath.Join(repo, DefaultWorkflowPolicy)
	if err := os.WriteFile(proposalPath, []byte(`{"states":{"todo":["done"]}}`), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}
	signer := newTestSigner(t)
	lock, err := GenerateWorkflowLock(repo, DefaultWorkflowPolicy, "signer-local", signer)
	if err != nil {
		t.Fatalf("generate lock failed: %v", err)
	}
	lock.Signature.Value = base64.StdEncoding.EncodeToString([]byte("bad-signature"))
	if err := ValidateWorkflowLock(repo, lock); !errors.Is(err, ErrWorkflowLockSignature) {
		t.Fatalf("expected signature validation error, got: %v", err)
	}
}

func TestWorkflowLockSemanticsSupportsCustomVerificationState(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	proposalPath := filepath.Join(repo, DefaultWorkflowPolicy)
	proposal := `{
		"states":{
			"todo":["doing"],
			"doing":["ready-for-qa"],
			"ready-for-qa":["closed"],
			"closed":[]
		},
		"semantics":{
			"review":["ready-for-qa"],
			"verification":["ready-for-qa"],
			"closure":["closed"],
			"human_only_closure":true
		}
	}`
	if err := os.WriteFile(proposalPath, []byte(proposal), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}
	signer := newTestSigner(t)
	lock, err := GenerateWorkflowLock(repo, DefaultWorkflowPolicy, "signer-local", signer)
	if err != nil {
		t.Fatalf("generate lock failed: %v", err)
	}
	if err := ValidateWorkflowLock(repo, lock); err != nil {
		t.Fatalf("validate lock failed: %v", err)
	}
}

func TestWorkflowLockSemanticsRejectsInvariantViolations(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	proposalPath := filepath.Join(repo, DefaultWorkflowPolicy)
	proposal := `{
		"states":{
			"todo":["doing"],
			"doing":["closed"],
			"ready-for-qa":["closed"],
			"closed":[]
		},
		"semantics":{
			"review":["ready-for-qa"],
			"verification":["ready-for-qa"],
			"closure":["closed"],
			"human_only_closure":false
		}
	}`
	if err := os.WriteFile(proposalPath, []byte(proposal), 0o644); err != nil {
		t.Fatalf("write proposal failed: %v", err)
	}
	signer := newTestSigner(t)
	_, err := GenerateWorkflowLock(repo, DefaultWorkflowPolicy, "signer-local", signer)
	if !errors.Is(err, ErrWorkflowLockMalformed) {
		t.Fatalf("expected malformed lock error for semantic invariant violation, got: %v", err)
	}
}
