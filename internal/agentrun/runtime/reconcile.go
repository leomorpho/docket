package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/artifacts"
	"github.com/leomorpho/docket/internal/runstate"
	"github.com/leomorpho/docket/internal/ticket"
)

const RecoverableStatusStaleAfter = 24 * time.Hour

type ReconciliationIssue struct {
	Kind        string `json:"kind"`
	TicketID    string `json:"ticket_id,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Path        string `json:"path,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	LastEventAt string `json:"last_event_at,omitempty"`
	LegacyState string `json:"legacy_state,omitempty"`
}

type ReconciliationResult struct {
	Applied       bool                  `json:"applied"`
	MutationCount int                   `json:"mutation_count"`
	Issues        []ReconciliationIssue `json:"issues,omitempty"`
}

type checkpointSnapshot struct {
	TicketID    string `json:"ticket_id"`
	TicketState string `json:"ticket_state,omitempty"`
}

func (s *Store) ScanReconciliationIssues(namespace *runstate.Store, now time.Time) ([]ReconciliationIssue, error) {
	if namespace == nil {
		return nil, fmt.Errorf("namespace store is required")
	}
	manifestMap, err := scanManifestTickets(namespace)
	if err != nil {
		return nil, err
	}

	issues := make([]ReconciliationIssue, 0)
	runIDs, err := s.ListRunTicketIDs()
	if err != nil {
		return nil, err
	}
	sort.Strings(runIDs)
	for _, ticketID := range runIDs {
		if _, ok := manifestMap[ticketID]; !ok {
			issues = append(issues, ReconciliationIssue{
				Kind:     "orphan_run_dir",
				TicketID: ticketID,
				Detail:   "runtime run dir exists without a matching namespace run manifest",
				Path:     s.RunDir(ticketID),
			})
		}

		status, ok, err := s.loadStatusRaw(ticketID)
		if err != nil {
			return nil, err
		}
		if !ok || !isRecoverableRuntimeStatus(status) {
			continue
		}

		brief, briefOK, err := s.LoadBrief(ticketID)
		if err != nil {
			return nil, err
		}
		if !briefOK {
			issues = append(issues, ReconciliationIssue{
				Kind:      "missing_brief",
				TicketID:  ticketID,
				SessionID: strings.TrimSpace(status.SessionID),
				Detail:    "recoverable runtime status does not have a durable brief",
				Path:      s.briefPath(ticketID),
			})
			continue
		}
		if !isStaleRecoverableStatus(status, now) {
			_ = brief
			continue
		}
		lastEventAt := latestRuntimeTimestamp(status)
		issues = append(issues, ReconciliationIssue{
			Kind:        "stale_recoverable_status",
			TicketID:    ticketID,
			SessionID:   strings.TrimSpace(status.SessionID),
			LastEventAt: lastEventAt,
			Detail:      fmt.Sprintf("recoverable runtime status has been inactive for at least %s", RecoverableStatusStaleAfter),
			Path:        filepath.Join(s.RunDir(ticketID), statusFile),
		})
	}

	checkpointIssues, err := s.scanLegacyCheckpointIssues()
	if err != nil {
		return nil, err
	}
	issues = append(issues, checkpointIssues...)

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].TicketID != issues[j].TicketID {
			return issues[i].TicketID < issues[j].TicketID
		}
		if issues[i].Kind != issues[j].Kind {
			return issues[i].Kind < issues[j].Kind
		}
		return issues[i].Path < issues[j].Path
	})
	return issues, nil
}

func (s *Store) ApplyReconciliation(namespace *runstate.Store, now time.Time) (ReconciliationResult, error) {
	issues, err := s.ScanReconciliationIssues(namespace, now)
	if err != nil {
		return ReconciliationResult{}, err
	}

	result := ReconciliationResult{
		Applied:       false,
		MutationCount: 0,
		Issues:        issues,
	}
	for _, issue := range issues {
		changed, err := s.applyReconciliationIssue(namespace, issue)
		if err != nil {
			return ReconciliationResult{}, err
		}
		if changed {
			result.Applied = true
			result.MutationCount++
		}
	}
	return result, nil
}

func scanManifestTickets(namespace *runstate.Store) (map[string]struct{}, error) {
	manifests, err := namespace.ListRunManifests()
	if err != nil {
		return nil, err
	}
	byTicket := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		if strings.TrimSpace(manifest.TicketID) == "" {
			continue
		}
		byTicket[manifest.TicketID] = struct{}{}
	}
	return byTicket, nil
}

func isRecoverableRuntimeStatus(status StatusSnapshot) bool {
	if strings.TrimSpace(status.SessionID) == "" {
		return false
	}
	if status.Hung {
		return true
	}
	switch strings.TrimSpace(status.LastResultStatus) {
	case string(agentrun.StatusFailed), string(agentrun.StatusStuck):
		return true
	default:
		return false
	}
}

func isStaleRecoverableStatus(status StatusSnapshot, now time.Time) bool {
	at := latestRuntimeTimestamp(status)
	if strings.TrimSpace(at) == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return false
	}
	return now.UTC().Sub(parsed.UTC()) >= RecoverableStatusStaleAfter
}

func latestRuntimeTimestamp(status StatusSnapshot) string {
	best := time.Time{}
	bestRaw := ""
	for _, raw := range []string{status.LastVisibleAt, status.LastEventAt} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			continue
		}
		if bestRaw == "" || parsed.After(best) {
			best = parsed
			bestRaw = raw
		}
	}
	return bestRaw
}

func (s *Store) scanLegacyCheckpointIssues() ([]ReconciliationIssue, error) {
	checkpointsRoot := artifacts.RepoPath(s.repoRoot, artifacts.RepoCheckpoints)
	entries, err := os.ReadDir(checkpointsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	issues := make([]ReconciliationIssue, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(checkpointsRoot, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var snapshot checkpointSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, err
		}
		legacyState := strings.TrimSpace(snapshot.TicketState)
		if legacyState == "" {
			continue
		}
		if ticket.MigrateWorkflowStateName(legacyState) == legacyState {
			continue
		}
		issues = append(issues, ReconciliationIssue{
			Kind:        "legacy_checkpoint",
			TicketID:    strings.TrimSpace(snapshot.TicketID),
			LegacyState: legacyState,
			Detail:      fmt.Sprintf("checkpoint still carries legacy ticket_state %s", legacyState),
			Path:        path,
		})
	}
	return issues, nil
}

func (s *Store) applyReconciliationIssue(namespace *runstate.Store, issue ReconciliationIssue) (bool, error) {
	switch issue.Kind {
	case "orphan_run_dir":
		return removePathIfExists(issue.Path)
	case "stale_recoverable_status":
		changed, err := s.cleanupManagedRuntime(namespace, issue.TicketID)
		if err != nil {
			return false, err
		}
		return changed, nil
	case "missing_brief":
		changed, err := s.cleanupManagedRuntime(namespace, issue.TicketID)
		if err != nil {
			return false, err
		}
		return changed, nil
	case "legacy_checkpoint":
		return rewriteLegacyCheckpoint(issue.Path)
	default:
		return false, nil
	}
}

func (s *Store) cleanupManagedRuntime(namespace *runstate.Store, ticketID string) (bool, error) {
	changed := false

	runRemoved, err := removePathIfExists(s.RunDir(ticketID))
	if err != nil {
		return false, err
	}
	changed = changed || runRemoved

	if namespace != nil {
		manifestRemoved, err := removeRunManifestIfExists(namespace, s.repoRoot, ticketID)
		if err != nil {
			return false, err
		}
		changed = changed || manifestRemoved
	}
	return changed, nil
}

func removeRunManifestIfExists(namespace *runstate.Store, repoRoot, ticketID string) (bool, error) {
	_, ok, err := namespace.GetRunManifest(repoRoot, ticketID)
	if err != nil {
		if errors.Is(err, runstate.ErrRunManifestInvalid) {
			if removeErr := namespace.DeleteRunManifest(repoRoot, ticketID); removeErr != nil {
				return false, removeErr
			}
			return true, nil
		}
		return false, err
	}
	if !ok {
		return false, nil
	}
	if err := namespace.DeleteRunManifest(repoRoot, ticketID); err != nil {
		return false, err
	}
	return true, nil
}

func rewriteLegacyCheckpoint(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return false, err
	}
	rawState, _ := payload["ticket_state"].(string)
	legacyState := strings.TrimSpace(rawState)
	if legacyState == "" {
		return false, nil
	}
	nextState := ticket.MigrateWorkflowStateName(legacyState)
	if nextState == legacyState {
		return false, nil
	}
	payload["ticket_state"] = nextState
	rewritten, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, append(rewritten, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func removePathIfExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.RemoveAll(path); err != nil {
		return false, err
	}
	return true, nil
}
