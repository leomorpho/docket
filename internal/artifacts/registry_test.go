package artifacts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	docketgit "github.com/leomorpho/docket/internal/git"
)

func TestRegistryCoversCoreManagedArtifacts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key       Key
		root      Root
		policy    Policy
		migration Migration
		relPath   string
	}{
		{RepoConfigJSON, RootRepo, PolicyTracked, MigrationStable, ".docket/config.json"},
		{RepoConfigYAML, RootRepo, PolicyTracked, MigrationStable, ".docket/config.yaml"},
		{RepoTicketsDir, RootRepo, PolicyTracked, MigrationStable, ".docket/tickets"},
		{RepoCheckpoints, RootRepo, PolicyLocalOnly, MigrationStable, ".docket/checkpoints"},
		{RepoIndexDB, RootRepo, PolicyLocalOnly, MigrationMigrateToLocal, ".docket/index.db"},
		{RepoInstallManifest, RootRepo, PolicyTracked, MigrationRegenerate, ".docket/install.json"},
		{RepoLifecycleEvents, RootRepo, PolicyLocalOnly, MigrationMigrateToLocal, ".docket/runtime/lifecycle-events.jsonl"},
		{RepoLearnRules, RootRepo, PolicyLocalOnly, MigrationMigrateToLocal, ".docket/runtime/learn-rules.json"},
		{RepoSkillUsageEvents, RootRepo, PolicyLocalOnly, MigrationMigrateToLocal, ".docket/runtime/skill-usage.jsonl"},
		{RepoSemanticDir, RootRepo, PolicyLocalOnly, MigrationMigrateToLocal, ".docket/semantic"},
		{RepoWorkflowPolicy, RootRepo, PolicyTracked, MigrationStable, ".docket/workflow.proposal.json"},
		{RepoWorkflowLock, RootRepo, PolicyTracked, MigrationStable, ".docket/workflow.lock.json"},
		{RepoProofsDir, RootRepo, PolicyTracked, MigrationStable, ".docket/proofs"},
		{RepoRelations, RootRepo, PolicyTracked, MigrationStable, ".docket/relations.json"},
		{RepoLocks, RootRepo, PolicyLocalOnly, MigrationMigrateToLocal, ".docket/locks.json"},
		{RepoLedgerEvents, RootRepo, PolicyTracked, MigrationStable, ".docket/ledger/events.jsonl"},
		{HomeSecurityKeystore, RootHome, PolicyLocalOnly, MigrationStable, "security/keystore.json"},
		{HomeSecuritySession, RootHome, PolicyLocalOnly, MigrationStable, "security/session.json"},
		{HomeSecurityApprovals, RootHome, PolicyLocalOnly, MigrationStable, "security/approvals.log"},
		{HomeIdentity, RootHome, PolicyLocalOnly, MigrationStable, "identity/identity.json"},
		{HomeReposDir, RootHome, PolicyLocalOnly, MigrationStable, "repos"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.key), func(t *testing.T) {
			t.Parallel()

			entry, ok := Lookup(tc.key)
			if !ok {
				t.Fatalf("registry missing %s", tc.key)
			}
			if entry.Root != tc.root {
				t.Fatalf("root mismatch for %s: got %s want %s", tc.key, entry.Root, tc.root)
			}
			if entry.Policy != tc.policy {
				t.Fatalf("policy mismatch for %s: got %s want %s", tc.key, entry.Policy, tc.policy)
			}
			if entry.Migration != tc.migration {
				t.Fatalf("migration mismatch for %s: got %s want %s", tc.key, entry.Migration, tc.migration)
			}
			if entry.RelPath != tc.relPath {
				t.Fatalf("rel path mismatch for %s: got %s want %s", tc.key, entry.RelPath, tc.relPath)
			}
		})
	}
}

func TestRegistryHelpersResolvePathsAndIgnorePatterns(t *testing.T) {
	t.Parallel()

	repoRoot := "/repo"
	docketHome := "/home/user/.docket-home"

	if got, want := RepoPath(repoRoot, RepoLifecycleEvents), filepath.Join(repoRoot, ".docket", "runtime", "lifecycle-events.jsonl"); got != want {
		t.Fatalf("RepoPath() mismatch: got %s want %s", got, want)
	}
	if got, want := WriteRepoPath(repoRoot, RepoLifecycleEvents), filepath.Join(repoRoot, ".docket", "local", "runtime", "lifecycle-events.jsonl"); got != want {
		t.Fatalf("WriteRepoPath() mismatch: got %s want %s", got, want)
	}
	if got, want := RepoPath(repoRoot, RepoProofsDir, "TKT-123", "metadata.json"), filepath.Join(repoRoot, ".docket", "proofs", "TKT-123", "metadata.json"); got != want {
		t.Fatalf("RepoPath() nested mismatch: got %s want %s", got, want)
	}
	if got, want := HomePath(docketHome, HomeReposDir, "drid_123", "runs", "TKT-300.json"), filepath.Join(docketHome, "repos", "drid_123", "runs", "TKT-300.json"); got != want {
		t.Fatalf("HomePath() mismatch: got %s want %s", got, want)
	}

	ignores := RepoLocalIgnorePatterns()
	for _, want := range []string{
		".docket/checkpoints/",
		".docket/index.db",
		".docket/locks.json",
		".docket/runtime/",
		".docket/semantic/",
		".docket/tickets/*/sessions/",
	} {
		if !contains(ignores, want) {
			t.Fatalf("missing ignore pattern %q from %v", want, ignores)
		}
	}
}

func TestReadRepoPathPrefersCanonicalThenLegacy(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	key := RepoLifecycleEvents
	canonical := CanonicalRepoPath(repoRoot, key)
	legacy := LegacyRepoPath(repoRoot, key)

	if got := ReadRepoPath(repoRoot, key); got != canonical {
		t.Fatalf("ReadRepoPath() without files = %q, want canonical %q", got, canonical)
	}

	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if got := ReadRepoPath(repoRoot, key); got != legacy {
		t.Fatalf("ReadRepoPath() with only legacy file = %q, want %q", got, legacy)
	}

	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatalf("mkdir canonical: %v", err)
	}
	if err := os.WriteFile(canonical, []byte("canonical"), 0o644); err != nil {
		t.Fatalf("write canonical: %v", err)
	}
	if got := ReadRepoPath(repoRoot, key); got != canonical {
		t.Fatalf("ReadRepoPath() with both files = %q, want %q", got, canonical)
	}
}

func TestRepoPathsResolveToSharedRootFromWorktree(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	runGitArtifact(t, repoRoot, "init")
	runGitArtifact(t, repoRoot, "config", "user.email", "test@example.com")
	runGitArtifact(t, repoRoot, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repoRoot, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGitArtifact(t, repoRoot, "add", ".")
	runGitArtifact(t, repoRoot, "commit", "-m", "seed")

	worktreePath := filepath.Join(repoRoot, "wt")
	runGitArtifact(t, repoRoot, "worktree", "add", "-b", "docket/test-paths", worktreePath)

	sharedRoot := docketgit.SharedRepoRoot(tmpDirCanonical(t, repoRoot))
	got := RepoPath(worktreePath, RepoConfigJSON)
	want := filepath.Join(sharedRoot, ".docket", "config.json")
	if got != want {
		t.Fatalf("RepoPath(worktree) = %q, want shared root %q", got, want)
	}

	got = CanonicalRepoPath(worktreePath, RepoLifecycleEvents)
	want = filepath.Join(sharedRoot, ".docket", "local", "runtime", "lifecycle-events.jsonl")
	if got != want {
		t.Fatalf("CanonicalRepoPath(worktree) = %q, want shared root %q", got, want)
	}
}

func TestLocalOnlyRepoArtifactsDeclareCanonicalLocalLayout(t *testing.T) {
	t.Parallel()

	if got, want := CanonicalLocalRootRelPath(), ".docket/local"; got != want {
		t.Fatalf("CanonicalLocalRootRelPath() = %q, want %q", got, want)
	}

	cases := []struct {
		key     Key
		current string
		target  string
	}{
		{RepoIndexDB, ".docket/index.db", ".docket/local/index.db"},
		{RepoLifecycleEvents, ".docket/runtime/lifecycle-events.jsonl", ".docket/local/runtime/lifecycle-events.jsonl"},
		{RepoLearnRules, ".docket/runtime/learn-rules.json", ".docket/local/runtime/learn-rules.json"},
		{RepoSkillUsageEvents, ".docket/runtime/skill-usage.jsonl", ".docket/local/runtime/skill-usage.jsonl"},
		{RepoRuntimeCapabilities, ".docket/runtime/capabilities.json", ".docket/local/runtime/capabilities.json"},
		{RepoSemanticDir, ".docket/semantic", ".docket/local/semantic"},
		{RepoLocks, ".docket/locks.json", ".docket/local/locks.json"},
		{RepoTicketSessions, ".docket/tickets", ".docket/local/tickets"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.key), func(t *testing.T) {
			t.Parallel()

			entry := MustLookup(tc.key)
			if entry.RelPath != tc.current {
				t.Fatalf("current rel path mismatch for %s: got %s want %s", tc.key, entry.RelPath, tc.current)
			}
			if entry.CanonicalRelPath != tc.target {
				t.Fatalf("canonical rel path mismatch for %s: got %s want %s", tc.key, entry.CanonicalRelPath, tc.target)
			}
			if entry.Policy != PolicyLocalOnly {
				t.Fatalf("%s should be local-only, got %s", tc.key, entry.Policy)
			}
			if !strings.HasPrefix(entry.CanonicalRelPath, CanonicalLocalRootRelPath()) {
				t.Fatalf("%s canonical path should live under %s, got %s", tc.key, CanonicalLocalRootRelPath(), entry.CanonicalRelPath)
			}
		})
	}
}

func TestCoreLifecycleSourcesUseArtifactRegistry(t *testing.T) {
	t.Parallel()

	files := []string{
		"cmd/install_shared.go",
		"cmd/init.go",
		"cmd/relations.go",
		"cmd/locks.go",
		"internal/lifecycle/events.go",
		"internal/skillusage/events.go",
		"internal/learning/learning.go",
		"internal/proof/repository.go",
		"internal/security/file_keystore.go",
		"internal/security/repo_namespace.go",
		"internal/security/session.go",
		"internal/semantic/metadata.go",
		"internal/ticket/config.go",
		"internal/workflow/policy_lock.go",
	}

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			src := string(raw)
			if !strings.Contains(src, "artifacts.") {
				t.Fatalf("%s should resolve managed artifacts through internal/artifacts", rel)
			}
			if strings.Contains(src, `".docket`) {
				t.Fatalf("%s still contains raw .docket path literals; route them through internal/artifacts", rel)
			}
		})
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func runGitArtifact(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func tmpDirCanonical(t *testing.T, path string) string {
	t.Helper()
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
