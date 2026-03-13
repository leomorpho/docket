package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	keystoreVersion  = 1
	kdfSaltBytes     = 16
	kdfIterations    = 200_000
	derivedKeyLength = 32
	nonceSize        = 12
)

type encryptedEnvelope struct {
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type plaintextStore struct {
	DevicePrivateKey string                   `json:"device_private_key"`
	TrustedSigners   map[string]TrustedSigner `json:"trusted_signers"`
	RepoAnchors      map[string]RepoAnchor    `json:"repo_anchors"`
}

type FileKeystore struct {
	path     string
	unlocked bool
	state    plaintextStore
	created  time.Time
	updated  time.Time
	key      []byte
	salt     []byte
}

func NewFileKeystore(docketHome string) *FileKeystore {
	return &FileKeystore{
		path: filepath.Join(docketHome, "security", "keystore.json"),
	}
}

func (k *FileKeystore) Path() string {
	return k.path
}

func (k *FileKeystore) IsUnlocked() bool {
	return k.unlocked
}

func (k *FileKeystore) Create(passphrase string) error {
	if _, err := os.Stat(k.path); err == nil {
		return fmt.Errorf("keystore already exists at %s", k.path)
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating device key: %w", err)
	}
	_ = pub

	k.state = plaintextStore{
		DevicePrivateKey: base64.StdEncoding.EncodeToString(priv),
		TrustedSigners:   map[string]TrustedSigner{},
		RepoAnchors:      map[string]RepoAnchor{},
	}
	k.salt = make([]byte, kdfSaltBytes)
	if _, err := rand.Read(k.salt); err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}
	k.key, err = deriveKey(passphrase, k.salt)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	k.created = now
	k.updated = now
	k.unlocked = true

	return k.Save()
}

func (k *FileKeystore) Unlock(passphrase string) error {
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}
	data, err := os.ReadFile(k.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrKeystoreNotFound
		}
		return err
	}

	var env encryptedEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("%w: invalid JSON", ErrKeystoreMalformed)
	}
	if env.Version != keystoreVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrKeystoreMalformed, env.Version)
	}

	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil || len(salt) == 0 {
		return fmt.Errorf("%w: bad salt", ErrKeystoreMalformed)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil || len(nonce) != nonceSize {
		return fmt.Errorf("%w: bad nonce", ErrKeystoreMalformed)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil || len(ciphertext) == 0 {
		return fmt.Errorf("%w: bad ciphertext", ErrKeystoreMalformed)
	}

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return err
	}
	plaintext, err := decrypt(key, nonce, ciphertext)
	if err != nil {
		return ErrWrongPassphrase
	}

	var state plaintextStore
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return fmt.Errorf("%w: bad decrypted payload", ErrKeystoreMalformed)
	}
	if state.DevicePrivateKey == "" {
		return fmt.Errorf("%w: missing device private key", ErrKeystoreMalformed)
	}
	if state.TrustedSigners == nil {
		state.TrustedSigners = map[string]TrustedSigner{}
	}
	if state.RepoAnchors == nil {
		state.RepoAnchors = map[string]RepoAnchor{}
	}

	k.state = state
	k.salt = salt
	k.key = key
	k.created, _ = time.Parse(time.RFC3339, env.CreatedAt)
	k.updated, _ = time.Parse(time.RFC3339, env.UpdatedAt)
	if k.created.IsZero() {
		k.created = time.Now().UTC()
	}
	if k.updated.IsZero() {
		k.updated = time.Now().UTC()
	}
	k.unlocked = true
	return nil
}

func (k *FileKeystore) Save() error {
	if !k.unlocked {
		return ErrLocked
	}
	if len(k.key) != derivedKeyLength {
		return fmt.Errorf("keystore encryption key is unavailable")
	}
	if len(k.salt) == 0 {
		return fmt.Errorf("keystore salt is unavailable")
	}

	payload, err := json.Marshal(k.state)
	if err != nil {
		return err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext, err := encrypt(k.key, nonce, payload)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if k.created.IsZero() {
		k.created = now
	}
	k.updated = now

	env := encryptedEnvelope{
		Version:    keystoreVersion,
		Salt:       base64.StdEncoding.EncodeToString(k.salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		CreatedAt:  k.created.Format(time.RFC3339),
		UpdatedAt:  k.updated.Format(time.RFC3339),
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(k.path), 0o755); err != nil {
		return err
	}
	tmp := k.path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.path)
}

func (k *FileKeystore) DevicePublicKey() (ed25519.PublicKey, error) {
	priv, err := k.privateKey()
	if err != nil {
		return nil, err
	}
	pub := make([]byte, ed25519.PublicKeySize)
	copy(pub, priv.Public().(ed25519.PublicKey))
	return pub, nil
}

func (k *FileKeystore) SignDevice(message []byte) ([]byte, error) {
	priv, err := k.privateKey()
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, message), nil
}

func (k *FileKeystore) SetTrustedSigner(signer TrustedSigner) error {
	if !k.unlocked {
		return ErrLocked
	}
	if signer.ID == "" {
		return fmt.Errorf("trusted signer ID is required")
	}
	if len(signer.Public) != ed25519.PublicKeySize {
		return fmt.Errorf("trusted signer public key must be %d bytes", ed25519.PublicKeySize)
	}
	if k.state.TrustedSigners == nil {
		k.state.TrustedSigners = map[string]TrustedSigner{}
	}
	k.state.TrustedSigners[signer.ID] = signer
	return nil
}

func (k *FileKeystore) GetTrustedSigner(id string) (TrustedSigner, bool, error) {
	if !k.unlocked {
		return TrustedSigner{}, false, ErrLocked
	}
	s, ok := k.state.TrustedSigners[id]
	return s, ok, nil
}

func (k *FileKeystore) SetRepoAnchor(anchor RepoAnchor) error {
	if !k.unlocked {
		return ErrLocked
	}
	if anchor.RepoID == "" {
		return fmt.Errorf("repo_id is required")
	}
	if anchor.SignerID == "" {
		return fmt.Errorf("signer_id is required")
	}
	if k.state.RepoAnchors == nil {
		k.state.RepoAnchors = map[string]RepoAnchor{}
	}
	k.state.RepoAnchors[anchor.RepoID] = anchor
	return nil
}

func (k *FileKeystore) GetRepoAnchor(repoID string) (RepoAnchor, bool, error) {
	if !k.unlocked {
		return RepoAnchor{}, false, ErrLocked
	}
	a, ok := k.state.RepoAnchors[repoID]
	return a, ok, nil
}

func (k *FileKeystore) privateKey() (ed25519.PrivateKey, error) {
	if !k.unlocked {
		return nil, ErrLocked
	}
	raw, err := base64.StdEncoding.DecodeString(k.state.DevicePrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid device private key encoding", ErrKeystoreMalformed)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: invalid device private key size", ErrKeystoreMalformed)
	}
	return ed25519.PrivateKey(raw), nil
}

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, passphrase, salt, kdfIterations, derivedKeyLength)
}

func encrypt(key, nonce, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, nil), nil
}

func decrypt(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// Basic integrity assertion on decrypted payload shape.
	if len(plaintext) == 0 || !hmac.Equal([]byte{plaintext[0]}, []byte{byte('{')}) {
		return nil, errors.New("decrypted payload does not look like JSON")
	}
	return plaintext, nil
}
