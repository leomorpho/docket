package security

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionUnlockExpiryAndRelock(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}

	mgr := NewSessionManager(home)
	repoRoot := filepath.Join(home, "repo")
	if err := mgr.Unlock(repoRoot, "pw-1", 150*time.Millisecond); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}

	active, _, err := mgr.Status(repoRoot)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !active {
		t.Fatalf("expected secure session to be active after unlock")
	}

	time.Sleep(220 * time.Millisecond)
	if err := mgr.RequireActive(repoRoot); !errors.Is(err, ErrSecureModeInactive) {
		t.Fatalf("expected inactive error after expiry, got: %v", err)
	}

	if err := mgr.Unlock(repoRoot, "pw-1", 2*time.Second); err != nil {
		t.Fatalf("re-unlock failed: %v", err)
	}
	if err := mgr.Lock(); err != nil {
		t.Fatalf("lock failed: %v", err)
	}
	if err := mgr.RequireActive(repoRoot); !errors.Is(err, ErrSecureModeInactive) {
		t.Fatalf("expected inactive error after lock, got: %v", err)
	}
}

func TestRecordPrivilegedActionRequiresActiveSession(t *testing.T) {
	home := t.TempDir()
	ks := NewFileKeystore(home)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}
	mgr := NewSessionManager(home)
	repoRoot := filepath.Join(home, "repo")

	if err := mgr.RecordPrivilegedAction(repoRoot, "TKT-001", "approve done transition"); !errors.Is(err, ErrSecureModeInactive) {
		t.Fatalf("expected secure-mode inactive rejection, got: %v", err)
	}

	if err := mgr.Unlock(repoRoot, "pw-1", time.Second); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	if err := mgr.RecordPrivilegedAction(repoRoot, "TKT-001", "approve done transition"); err != nil {
		t.Fatalf("record privileged action failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, "security", "approvals.log"))
	if err != nil {
		t.Fatalf("read approvals log failed: %v", err)
	}
	if !bytes.Contains(data, []byte("TKT-001")) {
		t.Fatalf("expected approvals log to include ticket ID, got: %s", string(data))
	}
}

func TestConfirmPrivilegedAction(t *testing.T) {
	var out bytes.Buffer
	ok, err := ConfirmPrivilegedAction(bytes.NewBufferString("yes\n"), &out, "/repo", "TKT-001", "set anchor")
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected yes confirmation")
	}
	if !bytes.Contains(out.Bytes(), []byte("ticket=TKT-001")) {
		t.Fatalf("expected prompt to identify ticket, got: %s", out.String())
	}
}

func TestRequireActiveRejectsRolledBackLedger(t *testing.T) {
	home := t.TempDir()
	repoRoot := filepath.Join(home, "repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir repo docket failed: %v", err)
	}

	ks := NewFileKeystore(home)
	if err := ks.Create("pw-1"); err != nil {
		t.Fatalf("create keystore failed: %v", err)
	}
	ledger := NewEventLedger(repoRoot, ks, "dev-test")
	if _, err := ledger.Append(LedgerAppendInput{
		Type:   EventRunStarted,
		RepoID: "drid_test",
		Actor:  "agent:test",
	}); err != nil {
		t.Fatalf("append first event failed: %v", err)
	}
	if _, err := ledger.Append(LedgerAppendInput{
		Type:   EventRunStopped,
		RepoID: "drid_test",
		Actor:  "agent:test",
	}); err != nil {
		t.Fatalf("append second event failed: %v", err)
	}

	ns := NewRepoNamespaceStore(home)
	if err := ns.VerifyAndAdvanceTrustedLedgerHead(repoRoot); err != nil {
		t.Fatalf("seed trusted head failed: %v", err)
	}

	data, err := os.ReadFile(LedgerPath(repoRoot))
	if err != nil {
		t.Fatalf("read ledger failed: %v", err)
	}
	lines := bytesSplitLines(data)
	if len(lines) != 2 {
		t.Fatalf("expected two ledger events, got %d", len(lines))
	}
	if err := os.WriteFile(LedgerPath(repoRoot), append(lines[0], '\n'), 0o644); err != nil {
		t.Fatalf("write rolled-back ledger failed: %v", err)
	}

	mgr := NewSessionManager(home)
	if err := mgr.Unlock(repoRoot, "pw-1", time.Second); err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	if err := mgr.RequireActive(repoRoot); !errors.Is(err, ErrLedgerHeadRollback) {
		t.Fatalf("expected rollback rejection in privileged flow, got: %v", err)
	}
}
