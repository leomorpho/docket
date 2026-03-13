package security

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileKeystoreCreateUnlockReadWrite(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)

	if err := ks.Create("pass-123"); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !ks.IsUnlocked() {
		t.Fatalf("expected keystore to be unlocked after create")
	}
	if _, err := os.Stat(ks.Path()); err != nil {
		t.Fatalf("expected keystore file at %s: %v", ks.Path(), err)
	}
	if !filepath.IsAbs(ks.Path()) || filepath.Dir(filepath.Dir(ks.Path())) != home {
		t.Fatalf("keystore path should be rooted under DOCKET_HOME, got: %s", ks.Path())
	}

	pub, err := ks.DevicePublicKey()
	if err != nil {
		t.Fatalf("device public key failed: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("unexpected public key size: %d", len(pub))
	}

	msg := []byte("sign me")
	sig, err := ks.SignDevice(msg)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if !ed25519.Verify(pub, msg, sig) {
		t.Fatalf("signature verification failed")
	}

	trustedPub := make([]byte, ed25519.PublicKeySize)
	copy(trustedPub, pub)
	if err := ks.SetTrustedSigner(TrustedSigner{
		ID:     "device-1",
		Public: trustedPub,
		Metadata: map[string]string{
			"name": "local-device",
		},
	}); err != nil {
		t.Fatalf("set trusted signer failed: %v", err)
	}
	if err := ks.SetRepoAnchor(RepoAnchor{RepoID: "repo-abc", SignerID: "device-1"}); err != nil {
		t.Fatalf("set repo anchor failed: %v", err)
	}
	if err := ks.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded := NewFileKeystore(home)
	if err := loaded.Unlock("pass-123"); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	if got, ok, err := loaded.GetTrustedSigner("device-1"); err != nil || !ok {
		t.Fatalf("get trusted signer failed: ok=%v err=%v", ok, err)
	} else if got.Metadata["name"] != "local-device" {
		t.Fatalf("unexpected signer metadata: %#v", got.Metadata)
	}
	if got, ok, err := loaded.GetRepoAnchor("repo-abc"); err != nil || !ok {
		t.Fatalf("get repo anchor failed: ok=%v err=%v", ok, err)
	} else if got.SignerID != "device-1" {
		t.Fatalf("unexpected repo anchor signer: %s", got.SignerID)
	}
}

func TestFileKeystoreUnlockWrongPassphrase(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)
	if err := ks.Create("correct-pass"); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	loaded := NewFileKeystore(home)
	if err := loaded.Unlock("wrong-pass"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("expected ErrWrongPassphrase, got: %v", err)
	}
}

func TestFileKeystoreMalformed(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)
	if err := os.MkdirAll(filepath.Dir(ks.Path()), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(ks.Path(), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	err := ks.Unlock("any-pass")
	if !errors.Is(err, ErrKeystoreMalformed) {
		t.Fatalf("expected ErrKeystoreMalformed, got: %v", err)
	}
}

func TestFileKeystoreLockedGuards(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)

	if _, err := ks.SignDevice([]byte("x")); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked from SignDevice, got: %v", err)
	}
	if err := ks.Save(); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked from Save, got: %v", err)
	}
	if _, ok, err := ks.GetTrustedSigner("id"); !errors.Is(err, ErrLocked) || ok {
		t.Fatalf("expected locked get trusted signer, got ok=%v err=%v", ok, err)
	}
}

func TestFileKeystoreEncryptsPayload(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)
	if err := ks.Create("pass-123"); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	raw, err := os.ReadFile(ks.Path())
	if err != nil {
		t.Fatalf("read keystore failed: %v", err)
	}
	if bytes.Contains(raw, []byte("device_private_key")) {
		t.Fatalf("keystore file should not contain plaintext private key payload")
	}
}
