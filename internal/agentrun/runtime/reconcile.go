package runtime

import (
	"encoding/json"
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
