package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/runstate"
)

func TestStoreScanReconciliationIssuesDetectsRuntimeDamage(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := New(repoRoot)
	namespace := runstate.New(filepath.Join(repoRoot, ".docket", "local", "namespace"))
	now := time.Now().UTC()

	orphanRecord := agentrun.RunRecord{
		TicketID:     "TKT-611",
		Role:         agentrun.RoleImplementer,
		RepoRoot:     repoRoot,
		WorktreePath: filepath.Join(repoRoot, "wt", "TKT-611"),
		Branch:       "docket/TKT-611",
		SessionID:    "session-TKT-611",
	}
	if err := store.Init(orphanRecord, "orphan prompt", 10*time.Minute); err != nil {
		t.Fatalf("Init(orphan) error = %v", err)
	}
	if err := store.WriteStatus(StatusSnapshot{
		TicketID:         orphanRecord.TicketID,
		SessionID:        orphanRecord.SessionID,
		Active:           false,
		LastResultStatus: string(agentrun.StatusFailed),
		LastEventAt:      now.Add(-12 * time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("WriteStatus(orphan) error = %v", err)
	}

	if err := seedManagedRuntimeArtifactForScanTest(repoRoot, namespace, store, "TKT-612", StatusSnapshot{
		TicketID:         "TKT-612",
		SessionID:        "session-TKT-612",
		Active:           false,
		Hung:             true,
		LastResultStatus: string(agentrun.StatusFailed),
		LastEventAt:      now.Add(-72 * time.Hour).Format(time.RFC3339Nano),
		LastVisibleAt:    now.Add(-72 * time.Hour).Format(time.RFC3339Nano),
	}, true); err != nil {
		t.Fatalf("seed stale runtime artifact: %v", err)
	}

	if err := seedManagedRuntimeArtifactForScanTest(repoRoot, namespace, store, "TKT-613", StatusSnapshot{
		TicketID:         "TKT-613",
		SessionID:        "session-TKT-613",
		Active:           false,
		LastResultStatus: string(agentrun.StatusStuck),
		LastEventAt:      now.Format(time.RFC3339Nano),
	}, false); err != nil {
		t.Fatalf("seed missing brief artifact: %v", err)
	}

	if err := writeLegacyCheckpointForScanTest(repoRoot, "TKT-614", "done"); err != nil {
		t.Fatalf("write legacy checkpoint: %v", err)
	}

	issues, err := store.ScanReconciliationIssues(namespace, now)
	if err != nil {
		t.Fatalf("ScanReconciliationIssues() error = %v", err)
	}
	if len(issues) < 4 {
		t.Fatalf("expected at least 4 issues, got %#v", issues)
	}

	byKind := map[string]ReconciliationIssue{}
	for _, issue := range issues {
		byKind[issue.Kind] = issue
	}

	if issue := byKind["orphan_run_dir"]; issue.TicketID != "TKT-611" {
		t.Fatalf("expected orphan issue for TKT-611, got %#v", issue)
	}
	if issue := byKind["stale_recoverable_status"]; issue.TicketID != "TKT-612" {
		t.Fatalf("expected stale recoverable issue for TKT-612, got %#v", issue)
	}
	if issue := byKind["missing_brief"]; issue.TicketID != "TKT-613" {
		t.Fatalf("expected missing brief issue for TKT-613, got %#v", issue)
	}
	if issue := byKind["legacy_checkpoint"]; issue.TicketID != "TKT-614" || issue.LegacyState != "done" {
		t.Fatalf("expected legacy checkpoint issue for TKT-614/done, got %#v", issue)
	}
}

func seedManagedRuntimeArtifactForScanTest(repoRoot string, namespace *runstate.Store, store *Store, ticketID string, status StatusSnapshot, writeBrief bool) error {
	worktreePath := filepath.Join(repoRoot, "wt", ticketID)
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		return err
	}
	if err := namespace.RecordRunStart(repoRoot, ticketID, "agent:test", worktreePath, "docket/"+ticketID, "workflow-"+ticketID); err != nil {
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
	if err := store.Init(record, "managed prompt", 10*time.Minute); err != nil {
		return err
	}
	if err := store.WriteStatus(status); err != nil {
		return err
	}
	if !writeBrief {
		return nil
	}
	return store.WriteBrief(RunBrief{
		TicketID:   ticketID,
		Outcome:    string(agentrun.StatusFailed),
		Summary:    "Managed run failed and left recoverable runtime state.",
		SessionID:  status.SessionID,
		ResumeNext: "Inspect runtime artifacts before resuming the ticket.",
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func writeLegacyCheckpointForScanTest(repoRoot, ticketID, state string) error {
	path := filepath.Join(repoRoot, ".docket", "checkpoints", ticketID+"-legacy.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"ticket_id":    ticketID,
		"ticket_state": state,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
