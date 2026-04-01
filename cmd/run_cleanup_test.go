package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	runruntime "github.com/leomorpho/docket/internal/agentrun/runtime"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/ticket"
)

type runtimeTreeEntry struct {
	IsDir bool
	Data  string
}

type runCleanupFixture struct {
	orphanTicketID           string
	staleRecoverableTicketID string
	missingBriefTicketID     string
	legacyCheckpointTicketID string
	namespaceRoot            string
	runtimeRoot              string
	checkpointsRoot          string
}

func TestRunCleanupDryRunReportsRuntimeArtifactsWithoutMutatingRepo(t *testing.T) {
	h := newFakeRepoHarness(t)
	t.Setenv("DOCKET_HOME", "")
	docketHome = ""

	fixture := seedRunCleanupFixture(t, h)
	before := snapshotRuntimeTrees(t, fixture.runtimeRoot, fixture.namespaceRoot, fixture.checkpointsRoot)

	out, err := h.run("run-cleanup", "--dry-run")

	after := snapshotRuntimeTrees(t, fixture.runtimeRoot, fixture.namespaceRoot, fixture.checkpointsRoot)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected dry-run to leave runtime artifacts unchanged\nbefore=%#v\nafter=%#v", before, after)
	}
	if err != nil {
		t.Fatalf("run-cleanup --dry-run failed: %v\n%s", err, out)
	}

	for _, want := range []string{
		"Runtime cleanup dry-run.",
		fixture.orphanTicketID,
		"orphan run dir",
		fixture.staleRecoverableTicketID,
		"stale recoverable status",
		fixture.missingBriefTicketID,
		"missing durable brief",
		fixture.legacyCheckpointTicketID,
		"legacy checkpoint",
		"done",
		"No mutations applied.",
		"Apply with: docket run-cleanup --apply",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected dry-run output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRunCleanupDryRunJSONReportsStructuredIssuesWithoutMutation(t *testing.T) {
	h := newFakeRepoHarness(t)
	t.Setenv("DOCKET_HOME", "")
	docketHome = ""

	fixture := seedRunCleanupFixture(t, h)
	before := snapshotRuntimeTrees(t, fixture.runtimeRoot, fixture.namespaceRoot, fixture.checkpointsRoot)

	out, err := h.run("--format", "json", "run-cleanup", "--dry-run")

	after := snapshotRuntimeTrees(t, fixture.runtimeRoot, fixture.namespaceRoot, fixture.checkpointsRoot)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected dry-run json mode to leave runtime artifacts unchanged\nbefore=%#v\nafter=%#v", before, after)
	}
	if err != nil {
		t.Fatalf("run-cleanup --dry-run --format json failed: %v\n%s", err, out)
	}

	jsonOut, extractErr := extractFirstJSONObject(out)
	if extractErr != nil {
		t.Fatalf("extract json failed: %v\n%s", extractErr, out)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("unmarshal cleanup json failed: %v\n%s", err, out)
	}
	if payload["mode"] != "dry-run" {
		t.Fatalf("expected mode=dry-run, got %#v", payload["mode"])
	}
	if payload["applied"] != false {
		t.Fatalf("expected applied=false for dry-run, got %#v", payload["applied"])
	}
	if payload["mutation_count"] != float64(0) {
		t.Fatalf("expected mutation_count=0, got %#v", payload["mutation_count"])
	}

	issues, ok := payload["issues"].([]any)
	if !ok || len(issues) < 4 {
		t.Fatalf("expected at least 4 runtime cleanup issues, got %#v", payload["issues"])
	}

	byKind := map[string]map[string]any{}
	for _, raw := range issues {
		issue, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected issue object, got %#v", raw)
		}
		kind, _ := issue["kind"].(string)
		byKind[kind] = issue
	}

	if issue := byKind["orphan_run_dir"]; issue == nil || issue["ticket_id"] != fixture.orphanTicketID {
		t.Fatalf("expected orphan_run_dir issue for %s, got %#v", fixture.orphanTicketID, byKind["orphan_run_dir"])
	}
	if issue := byKind["stale_recoverable_status"]; issue == nil || issue["ticket_id"] != fixture.staleRecoverableTicketID {
		t.Fatalf("expected stale_recoverable_status issue for %s, got %#v", fixture.staleRecoverableTicketID, byKind["stale_recoverable_status"])
	}
	if issue := byKind["missing_brief"]; issue == nil || issue["ticket_id"] != fixture.missingBriefTicketID {
		t.Fatalf("expected missing_brief issue for %s, got %#v", fixture.missingBriefTicketID, byKind["missing_brief"])
	}
	legacy := byKind["legacy_checkpoint"]
	if legacy == nil {
		t.Fatalf("expected legacy_checkpoint issue, got %#v", byKind)
	}
	if legacy["ticket_id"] != fixture.legacyCheckpointTicketID {
		t.Fatalf("expected legacy checkpoint issue for %s, got %#v", fixture.legacyCheckpointTicketID, legacy)
	}
	if legacy["legacy_state"] != "done" {
		t.Fatalf("expected legacy checkpoint to report legacy_state done, got %#v", legacy)
	}
}

func seedRunCleanupFixture(t *testing.T, h *fakeRepoHarness) runCleanupFixture {
	t.Helper()

	fixture := runCleanupFixture{
		orphanTicketID:           "TKT-611",
		staleRecoverableTicketID: "TKT-612",
		missingBriefTicketID:     "TKT-613",
		legacyCheckpointTicketID: "TKT-614",
		namespaceRoot:            runtimeNamespaceRoot(h.repo),
		runtimeRoot:              filepath.Join(h.repo, ".docket", "local", "runtime"),
		checkpointsRoot:          filepath.Join(h.repo, ".docket", "checkpoints"),
	}

	h.seedTicket(fixture.staleRecoverableTicketID, 612, ticket.State("running"), updateRunnableAC())
	h.seedTicket(fixture.missingBriefTicketID, 613, ticket.State("running"), updateRunnableAC())
	h.seedTicket(fixture.legacyCheckpointTicketID, 614, ticket.State("validated"), updateRunnableAC())

	runtimeStore := runruntime.New(h.repo)
	namespaceStore := runstate.New(fixture.namespaceRoot)

	orphanRecord := agentrun.RunRecord{
		TicketID:     fixture.orphanTicketID,
		Role:         agentrun.RoleImplementer,
		RepoRoot:     h.repo,
		WorktreePath: filepath.Join(h.repo, "wt", fixture.orphanTicketID),
		Branch:       "docket/" + fixture.orphanTicketID,
		SessionID:    "session-" + fixture.orphanTicketID,
	}
	if err := runtimeStore.Init(orphanRecord, "orphan prompt", 10*time.Minute); err != nil {
		t.Fatalf("init orphan runtime: %v", err)
	}
	if err := runtimeStore.WriteStatus(runruntime.StatusSnapshot{
		TicketID:         fixture.orphanTicketID,
		SessionID:        orphanRecord.SessionID,
		Active:           false,
		LastResultStatus: string(agentrun.StatusFailed),
		LastEventAt:      time.Now().UTC().Add(-12 * time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write orphan status: %v", err)
	}

	staleTime := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	if err := seedManagedRuntimeArtifact(t, h.repo, namespaceStore, runtimeStore, fixture.staleRecoverableTicketID, runruntime.StatusSnapshot{
		TicketID:         fixture.staleRecoverableTicketID,
		SessionID:        "session-" + fixture.staleRecoverableTicketID,
		Active:           false,
		Hung:             true,
		LastResultStatus: string(agentrun.StatusFailed),
		LastEventAt:      staleTime,
		LastVisibleAt:    staleTime,
	}, true); err != nil {
		t.Fatalf("seed stale recoverable runtime: %v", err)
	}

	if err := seedManagedRuntimeArtifact(t, h.repo, namespaceStore, runtimeStore, fixture.missingBriefTicketID, runruntime.StatusSnapshot{
		TicketID:         fixture.missingBriefTicketID,
		SessionID:        "session-" + fixture.missingBriefTicketID,
		Active:           false,
		LastResultStatus: string(agentrun.StatusStuck),
		LastEventAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}, false); err != nil {
		t.Fatalf("seed missing brief runtime: %v", err)
	}

	writeLegacyCheckpointFixture(t, h.repo, fixture.legacyCheckpointTicketID, "done")
	return fixture
}

func seedManagedRuntimeArtifact(t *testing.T, repoRoot string, namespaceStore *runstate.Store, runtimeStore *runruntime.Store, ticketID string, status runruntime.StatusSnapshot, writeBrief bool) error {
	t.Helper()

	worktreePath := filepath.Join(repoRoot, "wt", ticketID)
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		return err
	}
	if err := namespaceStore.RecordRunStart(repoRoot, ticketID, "agent:test", worktreePath, "docket/"+ticketID, "workflow-"+ticketID); err != nil {
		return err
	}
	record := agentrun.RunRecord{
		TicketID:     ticketID,
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: worktreePath,
		Branch:       "docket/" + ticketID,
		SessionID:    status.SessionID,
	}
	if err := runtimeStore.Init(record, "managed prompt", 10*time.Minute); err != nil {
		return err
	}
	if err := runtimeStore.WriteStatus(status); err != nil {
		return err
	}
	if !writeBrief {
		return nil
	}
	return runtimeStore.WriteBrief(runruntime.RunBrief{
		TicketID:         ticketID,
		Outcome:          "failed",
		Summary:          "Managed run failed and left recoverable runtime state.",
		SessionID:        status.SessionID,
		ValidationErrors: []string{"still waiting on runtime reconciliation"},
		ResumeNext:       "Inspect runtime artifacts before resuming the ticket.",
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func writeLegacyCheckpointFixture(t *testing.T, repoRoot, ticketID, legacyState string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(repoRoot, ".docket", "checkpoints"), 0o755); err != nil {
		t.Fatalf("mkdir checkpoints: %v", err)
	}
	path := filepath.Join(repoRoot, ".docket", "checkpoints", ticketID+"-20260331T120000Z.json")
	payload := checkpoint{
		TicketID:     ticketID,
		TicketState:  legacyState,
		CreatedAt:    "2026-03-31T12:00:00Z",
		ACDone:       1,
		ACTotal:      2,
		ChangedFiles: []string{"feature.txt"},
		LastComments: []string{"legacy checkpoint state should be rewritten"},
		Branch:       "docket/" + ticketID,
		WorktreePath: repoRoot,
		Summary:      "Legacy checkpoint fixture",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy checkpoint: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy checkpoint: %v", err)
	}
	return path
}

func snapshotRuntimeTrees(t *testing.T, roots ...string) map[string]runtimeTreeEntry {
	t.Helper()

	snapshot := map[string]runtimeTreeEntry{}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", root, err)
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := filepath.Join(root, rel)
			entry := runtimeTreeEntry{IsDir: info.IsDir()}
			if !info.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				entry.Data = string(data)
			}
			snapshot[key] = entry
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return snapshot
}
