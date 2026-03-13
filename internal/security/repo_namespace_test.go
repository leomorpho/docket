package security

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepoNamespaceStableIDAndMoveSafety(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo-a")
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir repo docket failed: %v", err)
	}

	store := NewRepoNamespaceStore(home)
	repoID, nsDir, err := store.EnsureRepoNamespace(repo)
	if err != nil {
		t.Fatalf("ensure namespace failed: %v", err)
	}
	if !strings.HasPrefix(repoID, "drid_") {
		t.Fatalf("unexpected repo ID format: %s", repoID)
	}
	if _, err := os.Stat(filepath.Join(repo, ".docket", repoIDFile)); err != nil {
		t.Fatalf("expected repo ID file, got: %v", err)
	}
	if !strings.Contains(nsDir, filepath.Join(home, "repos", repoID)) {
		t.Fatalf("unexpected namespace dir: %s", nsDir)
	}

	if _, err := store.SetTrustAnchor(repo, "signer-1"); err != nil {
		t.Fatalf("set trust anchor failed: %v", err)
	}

	movedRepo := filepath.Join(t.TempDir(), "repo-renamed")
	if err := os.Rename(repo, movedRepo); err != nil {
		t.Fatalf("rename repo failed: %v", err)
	}

	gotID, signerID, ok, err := store.GetTrustAnchor(movedRepo)
	if err != nil {
		t.Fatalf("get trust anchor after move failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected trust anchor to be found after move")
	}
	if gotID != repoID || signerID != "signer-1" {
		t.Fatalf("unexpected anchor after move: repoID=%s signerID=%s", gotID, signerID)
	}
}

func TestRepoNamespaceIsolationAcrossRepos(t *testing.T) {
	home := t.TempDir()
	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	_ = os.MkdirAll(filepath.Join(repoA, ".docket"), 0o755)
	_ = os.MkdirAll(filepath.Join(repoB, ".docket"), 0o755)

	store := NewRepoNamespaceStore(home)
	idA, err := store.SetTrustAnchor(repoA, "signer-a")
	if err != nil {
		t.Fatalf("set anchor repo A failed: %v", err)
	}
	idB, err := store.SetTrustAnchor(repoB, "signer-b")
	if err != nil {
		t.Fatalf("set anchor repo B failed: %v", err)
	}
	if idA == idB {
		t.Fatalf("expected unique repo IDs per repo, got same %s", idA)
	}

	_, signerA, ok, err := store.GetTrustAnchorByRepoID(idA)
	if err != nil || !ok {
		t.Fatalf("get anchor A failed: ok=%v err=%v", ok, err)
	}
	_, signerB, ok, err := store.GetTrustAnchorByRepoID(idB)
	if err != nil || !ok {
		t.Fatalf("get anchor B failed: ok=%v err=%v", ok, err)
	}
	if signerA != "signer-a" || signerB != "signer-b" {
		t.Fatalf("unexpected signers: A=%s B=%s", signerA, signerB)
	}
}

func TestRunManifestRecordAndVerify(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo-a")
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir repo docket failed: %v", err)
	}

	store := NewRepoNamespaceStore(home)
	if err := store.RecordRunStart(repo, "TKT-197", "agent:test", "/tmp/wt-TKT-197", "docket/TKT-197", "hash-123"); err != nil {
		t.Fatalf("record run manifest failed: %v", err)
	}
	rec, ok, err := store.GetRunManifest(repo, "TKT-197")
	if err != nil || !ok {
		t.Fatalf("get run manifest failed: ok=%v err=%v", ok, err)
	}
	if rec.ActorType != "agent" || rec.Actor != "agent:test" {
		t.Fatalf("unexpected actor metadata: %#v", rec)
	}
	if rec.WorktreePath != "/tmp/wt-TKT-197" || rec.Branch != "docket/TKT-197" || rec.WorkflowHash != "hash-123" {
		t.Fatalf("unexpected run manifest fields: %#v", rec)
	}
	if err := store.VerifyRunContext(repo, "TKT-197", "agent:test", "/tmp/wt-TKT-197", "docket/TKT-197", "hash-123"); err != nil {
		t.Fatalf("verify run context failed: %v", err)
	}
}

func TestRunManifestVerifyStaleAndMismatch(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo-a")
	if err := os.MkdirAll(filepath.Join(repo, ".docket"), 0o755); err != nil {
		t.Fatalf("mkdir repo docket failed: %v", err)
	}

	store := NewRepoNamespaceStore(home)
	if err := store.RecordRunStart(repo, "TKT-198", "agent:test", "/tmp/wt-TKT-198", "docket/TKT-198", "hash-abc"); err != nil {
		t.Fatalf("record run manifest failed: %v", err)
	}
	if err := store.VerifyRunContext(repo, "TKT-198", "agent:other", "/tmp/wt-TKT-198", "docket/TKT-198", "hash-abc"); !errors.Is(err, ErrRunContextMismatch) {
		t.Fatalf("expected run context mismatch, got: %v", err)
	}

	rec, ok, err := store.GetRunManifest(repo, "TKT-198")
	if err != nil || !ok {
		t.Fatalf("get run manifest failed: ok=%v err=%v", ok, err)
	}
	rec.StartedAt = time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	repoID, nsDir, err := store.EnsureRepoNamespace(repo)
	if err != nil {
		t.Fatalf("ensure namespace failed: %v", err)
	}
	rec.RepoID = repoID
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal stale manifest failed: %v", err)
	}
	manifestPath := filepath.Join(nsDir, "runs", "TKT-198.json")
	if err := os.WriteFile(manifestPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write stale manifest failed: %v", err)
	}
	if err := store.VerifyRunContext(repo, "TKT-198", "agent:test", "/tmp/wt-TKT-198", "docket/TKT-198", "hash-abc"); !errors.Is(err, ErrRunManifestStale) {
		t.Fatalf("expected stale run manifest error, got: %v", err)
	}
}
