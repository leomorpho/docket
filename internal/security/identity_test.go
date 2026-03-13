package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"
)

func TestIdentityEnrollRotateRevokeAndDuplicateRejection(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}

	m := NewIdentityManager(home)
	md, err := m.EnsureInitialized(ks)
	if err != nil {
		t.Fatalf("ensure initialized failed: %v", err)
	}
	if md.CurrentDeviceID == "" || md.UserID == "" {
		t.Fatalf("identity metadata missing bootstrap fields: %+v", md)
	}

	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	if err := m.EnrollDevice("dev_remote", pub2); err != nil {
		t.Fatalf("enroll device failed: %v", err)
	}
	if err := m.EnrollDevice("dev_remote_dup", pub2); !errors.Is(err, ErrDuplicatePublicKey) {
		t.Fatalf("expected duplicate key rejection, got: %v", err)
	}

	pub3, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	if err := m.RotateCurrentDevice("dev_new_local", pub3); err != nil {
		t.Fatalf("rotate current device failed: %v", err)
	}

	loaded, err := m.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.CurrentDeviceID != "dev_new_local" {
		t.Fatalf("expected current device to rotate, got: %s", loaded.CurrentDeviceID)
	}
	if loaded.Devices["dev_new_local"].Status != "active" {
		t.Fatalf("new device should be active")
	}
	if loaded.Devices[md.CurrentDeviceID].Status != "revoked" {
		t.Fatalf("old current device should be revoked after rotation")
	}

	if err := m.RevokeDevice("dev_remote"); err != nil {
		t.Fatalf("revoke enrolled device failed: %v", err)
	}
	loaded, _ = m.Load()
	if loaded.Devices["dev_remote"].Status != "revoked" {
		t.Fatalf("expected revoked status for dev_remote")
	}
}

func TestIdentityRecoveryPath(t *testing.T) {
	homeA := t.TempDir()
	ksA := NewFileKeystore(homeA)
	if err := ksA.Create("pw-1"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}
	mA := NewIdentityManager(homeA)
	if _, err := mA.EnsureInitialized(ksA); err != nil {
		t.Fatalf("ensure initialized failed: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "identity-export.json")
	if err := mA.ExportTo(exportPath); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	homeB := t.TempDir()
	mB := NewIdentityManager(homeB)
	if err := mB.RecoverFrom(exportPath); err != nil {
		t.Fatalf("recover failed: %v", err)
	}

	mdB, err := mB.Load()
	if err != nil {
		t.Fatalf("load recovered metadata failed: %v", err)
	}
	if mdB.UserID == "" || mdB.CurrentDeviceID == "" || len(mdB.Devices) == 0 {
		t.Fatalf("recovered metadata incomplete: %+v", mdB)
	}
}
