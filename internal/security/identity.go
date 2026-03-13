package security

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const identityVersion = 1

var (
	ErrDeviceExists       = errors.New("device already exists")
	ErrDeviceNotFound     = errors.New("device not found")
	ErrDuplicatePublicKey = errors.New("public key already enrolled")
)

type DeviceKeyMetadata struct {
	DeviceID  string `json:"device_id"`
	PublicKey string `json:"public_key"`
	AddedAt   string `json:"added_at"`
	RevokedAt string `json:"revoked_at,omitempty"`
	Status    string `json:"status"`
}

type IdentityMetadata struct {
	Version         int                          `json:"version"`
	UserID          string                       `json:"user_id"`
	CurrentDeviceID string                       `json:"current_device_id"`
	Devices         map[string]DeviceKeyMetadata `json:"devices"`
	RecoveryHints   map[string]string            `json:"recovery_hints,omitempty"`
	UpdatedAt       string                       `json:"updated_at"`
}

type IdentityManager struct {
	docketHome string
}

func NewIdentityManager(docketHome string) *IdentityManager {
	return &IdentityManager{docketHome: docketHome}
}

func (m *IdentityManager) Path() string {
	return filepath.Join(m.docketHome, "identity", "identity.json")
}

func (m *IdentityManager) EnsureInitialized(ks KeystoreProvider) (IdentityMetadata, error) {
	if md, err := m.Load(); err == nil {
		return md, nil
	}

	pub, err := ks.DevicePublicKey()
	if err != nil {
		return IdentityMetadata{}, err
	}
	deviceID := deriveDeviceID(pub)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	md := IdentityMetadata{
		Version:         identityVersion,
		UserID:          "usr_" + uuid.NewString(),
		CurrentDeviceID: deviceID,
		Devices: map[string]DeviceKeyMetadata{
			deviceID: {
				DeviceID:  deviceID,
				PublicKey: base64.StdEncoding.EncodeToString(pub),
				AddedAt:   now,
				Status:    "active",
			},
		},
		RecoveryHints: map[string]string{
			"export_command":  "docket secure identity export --path <file>",
			"recover_command": "docket secure identity recover --path <file>",
		},
		UpdatedAt: now,
	}
	if err := m.save(md); err != nil {
		return IdentityMetadata{}, err
	}
	return md, nil
}

func (m *IdentityManager) Load() (IdentityMetadata, error) {
	data, err := os.ReadFile(m.Path())
	if err != nil {
		return IdentityMetadata{}, err
	}
	var md IdentityMetadata
	if err := json.Unmarshal(data, &md); err != nil {
		return IdentityMetadata{}, err
	}
	if md.Version != identityVersion {
		return IdentityMetadata{}, fmt.Errorf("unsupported identity metadata version: %d", md.Version)
	}
	if md.Devices == nil {
		md.Devices = map[string]DeviceKeyMetadata{}
	}
	return md, nil
}

func (m *IdentityManager) ExportTo(path string) error {
	md, err := m.Load()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func (m *IdentityManager) RecoverFrom(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var md IdentityMetadata
	if err := json.Unmarshal(data, &md); err != nil {
		return err
	}
	if md.Version != identityVersion {
		return fmt.Errorf("unsupported identity metadata version: %d", md.Version)
	}
	if md.UserID == "" || len(md.Devices) == 0 {
		return fmt.Errorf("identity metadata is incomplete")
	}
	if _, ok := md.Devices[md.CurrentDeviceID]; !ok {
		return fmt.Errorf("current device %s missing from metadata", md.CurrentDeviceID)
	}
	md.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return m.save(md)
}

func (m *IdentityManager) EnrollDevice(deviceID string, publicKey ed25519.PublicKey) error {
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	md, err := m.Load()
	if err != nil {
		return err
	}
	if _, ok := md.Devices[deviceID]; ok {
		return ErrDeviceExists
	}
	encoded := base64.StdEncoding.EncodeToString(publicKey)
	for _, dev := range md.Devices {
		if dev.PublicKey == encoded {
			return ErrDuplicatePublicKey
		}
	}
	md.Devices[deviceID] = DeviceKeyMetadata{
		DeviceID:  deviceID,
		PublicKey: encoded,
		AddedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Status:    "active",
	}
	md.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return m.save(md)
}

func (m *IdentityManager) RevokeDevice(deviceID string) error {
	md, err := m.Load()
	if err != nil {
		return err
	}
	dev, ok := md.Devices[deviceID]
	if !ok {
		return ErrDeviceNotFound
	}
	if deviceID == md.CurrentDeviceID {
		return fmt.Errorf("cannot revoke current device without rotation")
	}
	dev.Status = "revoked"
	dev.RevokedAt = time.Now().UTC().Format(time.RFC3339Nano)
	md.Devices[deviceID] = dev
	md.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return m.save(md)
}

func (m *IdentityManager) RotateCurrentDevice(newDeviceID string, publicKey ed25519.PublicKey) error {
	if newDeviceID == "" {
		return fmt.Errorf("new device ID is required")
	}
	md, err := m.Load()
	if err != nil {
		return err
	}
	if _, ok := md.Devices[newDeviceID]; ok {
		return ErrDeviceExists
	}
	encoded := base64.StdEncoding.EncodeToString(publicKey)
	for _, dev := range md.Devices {
		if dev.PublicKey == encoded {
			return ErrDuplicatePublicKey
		}
	}

	old := md.Devices[md.CurrentDeviceID]
	old.Status = "revoked"
	old.RevokedAt = time.Now().UTC().Format(time.RFC3339Nano)
	md.Devices[md.CurrentDeviceID] = old

	md.Devices[newDeviceID] = DeviceKeyMetadata{
		DeviceID:  newDeviceID,
		PublicKey: encoded,
		AddedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Status:    "active",
	}
	md.CurrentDeviceID = newDeviceID
	md.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return m.save(md)
}

func deriveDeviceID(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return fmt.Sprintf("dev_%x", sum[:6])
}

func (m *IdentityManager) save(md IdentityMetadata) error {
	if err := os.MkdirAll(filepath.Dir(m.Path()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.Path(), append(data, '\n'), 0o600)
}
