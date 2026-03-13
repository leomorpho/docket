package security

import (
	"crypto/ed25519"
	"errors"
)

var (
	ErrLocked            = errors.New("keystore is locked")
	ErrWrongPassphrase   = errors.New("wrong passphrase")
	ErrKeystoreNotFound  = errors.New("keystore file not found")
	ErrKeystoreMalformed = errors.New("keystore file is malformed")
)

type TrustedSigner struct {
	ID       string            `json:"id"`
	Public   []byte            `json:"public"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type RepoAnchor struct {
	RepoID   string `json:"repo_id"`
	SignerID string `json:"signer_id"`
}

// KeystoreProvider defines the minimum operations required by the secure-mode roadmap.
// It deliberately exposes signing operations instead of raw key bytes.
type KeystoreProvider interface {
	Path() string
	Create(passphrase string) error
	Unlock(passphrase string) error
	IsUnlocked() bool
	Save() error

	DevicePublicKey() (ed25519.PublicKey, error)
	SignDevice(message []byte) ([]byte, error)

	SetTrustedSigner(signer TrustedSigner) error
	GetTrustedSigner(id string) (TrustedSigner, bool, error)

	SetRepoAnchor(anchor RepoAnchor) error
	GetRepoAnchor(repoID string) (RepoAnchor, bool, error)
}
