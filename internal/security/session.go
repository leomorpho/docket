package security

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrSecureModeInactive = errors.New("secure mode is inactive")

type sessionRecord struct {
	Repo       string `json:"repo"`
	UnlockedAt string `json:"unlocked_at"`
	ExpiresAt  string `json:"expires_at"`
}

type approvalRecord struct {
	At     string `json:"at"`
	Repo   string `json:"repo"`
	Ticket string `json:"ticket"`
	Action string `json:"action"`
}

type SessionManager struct {
	docketHome string
}

func NewSessionManager(docketHome string) *SessionManager {
	return &SessionManager{docketHome: docketHome}
}

func (m *SessionManager) sessionPath() string {
	return filepath.Join(m.docketHome, "security", "session.json")
}

func (m *SessionManager) approvalsPath() string {
	return filepath.Join(m.docketHome, "security", "approvals.log")
}

func (m *SessionManager) Unlock(repoRoot, passphrase string, ttl time.Duration) error {
	if repoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("ttl must be > 0")
	}

	ks := NewFileKeystore(m.docketHome)
	err := ks.Unlock(passphrase)
	if errors.Is(err, ErrKeystoreNotFound) {
		if err := ks.Create(passphrase); err != nil {
			return fmt.Errorf("creating keystore: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("unlocking keystore: %w", err)
	}

	now := time.Now().UTC()
	rec := sessionRecord{
		Repo:       repoRoot,
		UnlockedAt: now.Format(time.RFC3339Nano),
		ExpiresAt:  now.Add(ttl).Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.sessionPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.sessionPath(), append(data, '\n'), 0o600)
}

func (m *SessionManager) Lock() error {
	if err := os.Remove(m.sessionPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *SessionManager) Status(repoRoot string) (active bool, expiresAt time.Time, err error) {
	rec, err := m.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}
	exp, err := time.Parse(time.RFC3339Nano, rec.ExpiresAt)
	if err != nil {
		exp, err = time.Parse(time.RFC3339, rec.ExpiresAt)
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("parsing session expiry: %w", err)
	}
	if rec.Repo != repoRoot {
		return false, exp, nil
	}
	if !time.Now().UTC().Before(exp) {
		return false, exp, nil
	}
	return true, exp, nil
}

func (m *SessionManager) RequireActive(repoRoot string) error {
	active, _, err := m.Status(repoRoot)
	if err != nil {
		return err
	}
	if !active {
		return ErrSecureModeInactive
	}
	return nil
}

func (m *SessionManager) RecordPrivilegedAction(repoRoot, ticket, action string) error {
	if err := m.RequireActive(repoRoot); err != nil {
		return err
	}
	if ticket == "" {
		return fmt.Errorf("ticket is required")
	}
	if action == "" {
		return fmt.Errorf("action is required")
	}

	rec := approvalRecord{
		At:     time.Now().UTC().Format(time.RFC3339),
		Repo:   repoRoot,
		Ticket: ticket,
		Action: action,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.approvalsPath()), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(m.approvalsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func ConfirmPrivilegedAction(in io.Reader, out io.Writer, repoRoot, ticket, action string) (bool, error) {
	if ticket == "" || action == "" {
		return false, fmt.Errorf("ticket and action are required")
	}
	fmt.Fprintf(out, "Confirm privileged action for repo=%s ticket=%s action=%q [y/N]: ", repoRoot, ticket, action)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

func (m *SessionManager) load() (sessionRecord, error) {
	data, err := os.ReadFile(m.sessionPath())
	if err != nil {
		return sessionRecord{}, err
	}
	var rec sessionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return sessionRecord{}, err
	}
	return rec, nil
}
