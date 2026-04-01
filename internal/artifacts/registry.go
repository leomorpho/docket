package artifacts

import (
	"os"
	"path/filepath"
	"sort"

	docketgit "github.com/leomorpho/docket/internal/git"
)

type Key string
type Root string
type Policy string
type Migration string
type Category string
type Kind string

const (
	RootRepo Root = "repo"
	RootHome Root = "home"
)

const (
	PolicyTracked   Policy = "tracked"
	PolicyLocalOnly Policy = "local-only"
)

const (
	MigrationStable         Migration = "stable"
	MigrationMigrateToLocal Migration = "migrate-to-local"
	MigrationRegenerate     Migration = "regenerate"
)

const (
	KindFile Kind = "file"
	KindDir  Kind = "dir"
)

const (
	CategoryConfig    Category = "config"
	CategoryHooks     Category = "hooks"
	CategoryIndex     Category = "index"
	CategoryLearning  Category = "learning"
	CategoryProofs    Category = "proofs"
	CategoryRelations Category = "relations"
	CategoryRuntime   Category = "runtime"
	CategorySecurity  Category = "security"
	CategorySemantic  Category = "semantic"
	CategoryTickets   Category = "tickets"
	CategoryWorkflow  Category = "workflow"
)

const (
	RepoAdapter             Key = "repo.adapter"
	RepoCheckpoints         Key = "repo.checkpoints"
	RepoAgentRunsDir        Key = "repo.runtime.agent_runs"
	RepoConfigJSON          Key = "repo.config.json"
	RepoConfigYAML          Key = "repo.config.yaml"
	RepoIndexDB             Key = "repo.index.db"
	RepoInstallManifest     Key = "repo.install.manifest"
	RepoLifecycleEvents     Key = "repo.runtime.lifecycle_events"
	RepoLearnRules          Key = "repo.runtime.learn_rules"
	RepoSkillUsageEvents    Key = "repo.runtime.skill_usage_events"
	RepoLocks               Key = "repo.locks"
	RepoManifest            Key = "repo.manifest"
	RepoProofsDir           Key = "repo.proofs"
	RepoRelations           Key = "repo.relations"
	RepoRepoID              Key = "repo.repo_id"
	RepoRuntimeCapabilities Key = "repo.runtime.capabilities"
	RepoSemanticDir         Key = "repo.semantic"
	RepoTemplatesDir        Key = "repo.templates"
	RepoTicketsDir          Key = "repo.tickets"
	RepoTicketSessions      Key = "repo.tickets.sessions"
	RepoLedgerEvents        Key = "repo.security.ledger"
	HomeIdentity            Key = "home.identity"
	HomeSecurityKeystore    Key = "home.security.keystore"
	HomeSecuritySession     Key = "home.security.session"
	HomeSecurityApprovals   Key = "home.security.approvals"
	HomeReposDir            Key = "home.repos"
)

type Entry struct {
	Key              Key
	Category         Category
	Root             Root
	Kind             Kind
	RelPath          string
	CanonicalRelPath string
	Policy           Policy
	Migration        Migration
	IgnorePattern    string
}

var registry = map[Key]Entry{
	RepoAdapter:             {Key: RepoAdapter, Category: CategoryConfig, Root: RootRepo, Kind: KindFile, RelPath: ".docket/adapter", Policy: PolicyTracked, Migration: MigrationStable},
	RepoCheckpoints:         {Key: RepoCheckpoints, Category: CategoryRuntime, Root: RootRepo, Kind: KindDir, RelPath: ".docket/checkpoints", Policy: PolicyLocalOnly, Migration: MigrationStable, IgnorePattern: ".docket/checkpoints/"},
	RepoAgentRunsDir:        {Key: RepoAgentRunsDir, Category: CategoryRuntime, Root: RootRepo, Kind: KindDir, RelPath: ".docket/runtime/agent-runs", CanonicalRelPath: ".docket/local/runtime/agent-runs", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/runtime/"},
	RepoConfigJSON:          {Key: RepoConfigJSON, Category: CategoryConfig, Root: RootRepo, Kind: KindFile, RelPath: ".docket/config.json", Policy: PolicyTracked, Migration: MigrationStable},
	RepoConfigYAML:          {Key: RepoConfigYAML, Category: CategoryConfig, Root: RootRepo, Kind: KindFile, RelPath: ".docket/config.yaml", Policy: PolicyTracked, Migration: MigrationStable},
	RepoIndexDB:             {Key: RepoIndexDB, Category: CategoryIndex, Root: RootRepo, Kind: KindFile, RelPath: ".docket/index.db", CanonicalRelPath: ".docket/local/index.db", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/index.db"},
	RepoInstallManifest:     {Key: RepoInstallManifest, Category: CategoryHooks, Root: RootRepo, Kind: KindFile, RelPath: ".docket/install.json", Policy: PolicyTracked, Migration: MigrationRegenerate},
	RepoLifecycleEvents:     {Key: RepoLifecycleEvents, Category: CategoryRuntime, Root: RootRepo, Kind: KindFile, RelPath: ".docket/runtime/lifecycle-events.jsonl", CanonicalRelPath: ".docket/local/runtime/lifecycle-events.jsonl", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/runtime/"},
	RepoLearnRules:          {Key: RepoLearnRules, Category: CategoryLearning, Root: RootRepo, Kind: KindFile, RelPath: ".docket/runtime/learn-rules.json", CanonicalRelPath: ".docket/local/runtime/learn-rules.json", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/runtime/"},
	RepoSkillUsageEvents:    {Key: RepoSkillUsageEvents, Category: CategoryRuntime, Root: RootRepo, Kind: KindFile, RelPath: ".docket/runtime/skill-usage.jsonl", CanonicalRelPath: ".docket/local/runtime/skill-usage.jsonl", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/runtime/"},
	RepoLocks:               {Key: RepoLocks, Category: CategoryRelations, Root: RootRepo, Kind: KindFile, RelPath: ".docket/locks.json", CanonicalRelPath: ".docket/local/locks.json", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/locks.json"},
	RepoManifest:            {Key: RepoManifest, Category: CategoryTickets, Root: RootRepo, Kind: KindFile, RelPath: ".docket/manifest.json", Policy: PolicyTracked, Migration: MigrationStable},
	RepoProofsDir:           {Key: RepoProofsDir, Category: CategoryProofs, Root: RootRepo, Kind: KindDir, RelPath: ".docket/proofs", Policy: PolicyTracked, Migration: MigrationStable},
	RepoRelations:           {Key: RepoRelations, Category: CategoryRelations, Root: RootRepo, Kind: KindFile, RelPath: ".docket/relations.json", Policy: PolicyTracked, Migration: MigrationStable},
	RepoRepoID:              {Key: RepoRepoID, Category: CategorySecurity, Root: RootRepo, Kind: KindFile, RelPath: ".docket/repo_id", Policy: PolicyTracked, Migration: MigrationStable},
	RepoRuntimeCapabilities: {Key: RepoRuntimeCapabilities, Category: CategoryRuntime, Root: RootRepo, Kind: KindFile, RelPath: ".docket/runtime/capabilities.json", CanonicalRelPath: ".docket/local/runtime/capabilities.json", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/runtime/"},
	RepoSemanticDir:         {Key: RepoSemanticDir, Category: CategorySemantic, Root: RootRepo, Kind: KindDir, RelPath: ".docket/semantic", CanonicalRelPath: ".docket/local/semantic", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/semantic/"},
	RepoTemplatesDir:        {Key: RepoTemplatesDir, Category: CategoryConfig, Root: RootRepo, Kind: KindDir, RelPath: ".docket/templates", Policy: PolicyTracked, Migration: MigrationStable},
	RepoTicketsDir:          {Key: RepoTicketsDir, Category: CategoryTickets, Root: RootRepo, Kind: KindDir, RelPath: ".docket/tickets", Policy: PolicyTracked, Migration: MigrationStable},
	RepoTicketSessions:      {Key: RepoTicketSessions, Category: CategoryTickets, Root: RootRepo, Kind: KindDir, RelPath: ".docket/tickets", CanonicalRelPath: ".docket/local/tickets", Policy: PolicyLocalOnly, Migration: MigrationMigrateToLocal, IgnorePattern: ".docket/tickets/*/sessions/"},
	RepoLedgerEvents:        {Key: RepoLedgerEvents, Category: CategorySecurity, Root: RootRepo, Kind: KindFile, RelPath: ".docket/ledger/events.jsonl", Policy: PolicyTracked, Migration: MigrationStable},
	HomeIdentity:            {Key: HomeIdentity, Category: CategorySecurity, Root: RootHome, Kind: KindFile, RelPath: "identity/identity.json", Policy: PolicyLocalOnly, Migration: MigrationStable},
	HomeSecurityKeystore:    {Key: HomeSecurityKeystore, Category: CategorySecurity, Root: RootHome, Kind: KindFile, RelPath: "security/keystore.json", Policy: PolicyLocalOnly, Migration: MigrationStable},
	HomeSecuritySession:     {Key: HomeSecuritySession, Category: CategorySecurity, Root: RootHome, Kind: KindFile, RelPath: "security/session.json", Policy: PolicyLocalOnly, Migration: MigrationStable},
	HomeSecurityApprovals:   {Key: HomeSecurityApprovals, Category: CategorySecurity, Root: RootHome, Kind: KindFile, RelPath: "security/approvals.log", Policy: PolicyLocalOnly, Migration: MigrationStable},
	HomeReposDir:            {Key: HomeReposDir, Category: CategorySecurity, Root: RootHome, Kind: KindDir, RelPath: "repos", Policy: PolicyLocalOnly, Migration: MigrationStable},
}

func Lookup(key Key) (Entry, bool) {
	entry, ok := registry[key]
	return entry, ok
}

func MustLookup(key Key) Entry {
	entry, ok := Lookup(key)
	if !ok {
		panic("unknown artifact key: " + string(key))
	}
	return entry
}

func All() []Entry {
	out := make([]Entry, 0, len(registry))
	for _, entry := range registry {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func MustRelPath(key Key, more ...string) string {
	entry := MustLookup(key)
	parts := append([]string{entry.RelPath}, more...)
	return filepath.Join(parts...)
}

func RepoPath(repoRoot string, key Key, more ...string) string {
	entry := MustLookup(key)
	if entry.Root != RootRepo {
		panic("artifact is not repo-rooted: " + string(key))
	}
	repoRoot = docketgit.SharedRepoRoot(repoRoot)
	parts := append([]string{repoRoot, entry.RelPath}, more...)
	return filepath.Join(parts...)
}

func LegacyRepoPath(repoRoot string, key Key, more ...string) string {
	return RepoPath(repoRoot, key, more...)
}

func HomePath(docketHome string, key Key, more ...string) string {
	entry := MustLookup(key)
	if entry.Root != RootHome {
		panic("artifact is not docket-home rooted: " + string(key))
	}
	parts := append([]string{docketHome, entry.RelPath}, more...)
	return filepath.Join(parts...)
}

func RepoLocalIgnorePatterns() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, entry := range All() {
		if entry.Root != RootRepo || entry.Policy != PolicyLocalOnly || entry.IgnorePattern == "" {
			continue
		}
		if _, ok := seen[entry.IgnorePattern]; ok {
			continue
		}
		seen[entry.IgnorePattern] = struct{}{}
		out = append(out, entry.IgnorePattern)
	}
	sort.Strings(out)
	return out
}

func CanonicalLocalRootRelPath() string {
	return ".docket/local"
}

func CanonicalRepoPath(repoRoot string, key Key, more ...string) string {
	entry := MustLookup(key)
	if entry.Root != RootRepo {
		panic("artifact is not repo-rooted: " + string(key))
	}
	repoRoot = docketgit.SharedRepoRoot(repoRoot)
	base := entry.RelPath
	if entry.CanonicalRelPath != "" {
		base = entry.CanonicalRelPath
	}
	parts := append([]string{repoRoot, base}, more...)
	return filepath.Join(parts...)
}

func WriteRepoPath(repoRoot string, key Key, more ...string) string {
	return CanonicalRepoPath(repoRoot, key, more...)
}

func ReadRepoPath(repoRoot string, key Key, more ...string) string {
	canonical := CanonicalRepoPath(repoRoot, key, more...)
	if _, err := os.Stat(canonical); err == nil {
		return canonical
	}
	legacy := LegacyRepoPath(repoRoot, key, more...)
	if canonical != legacy {
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return canonical
}
