package cmd

import (
	"io"
	"sort"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/lifecycle"
	"github.com/leomorpho/docket/internal/runstate"
)

type transitionHistoryEntry struct {
	At          string   `json:"at"`
	Command     string   `json:"command"`
	Actor       string   `json:"actor"`
	FromState   string   `json:"from_state"`
	ToState     string   `json:"to_state"`
	Reason      string   `json:"reason"`
	Checks      []string `json:"checks,omitempty"`
	ManagedRun  bool     `json:"managed_run,omitempty"`
	RunBranch   string   `json:"run_branch,omitempty"`
	RunWorktree string   `json:"run_worktree,omitempty"`
}

func emitStateTransitionEvent(out io.Writer, command, ticketID, actor, fromState, toState, reason string, checks []string) {
	payload := map[string]any{
		"command":    strings.TrimSpace(command),
		"ticket_id":  strings.TrimSpace(ticketID),
		"actor":      strings.TrimSpace(actor),
		"from_state": strings.TrimSpace(fromState),
		"to_state":   strings.TrimSpace(toState),
		"reason":     strings.TrimSpace(reason),
		"checks":     normalizeChecks(checks),
	}

	ns := runstate.New(runtimeNamespaceRoot(repo))
	if run, ok, err := ns.GetRunManifest(repo, ticketID); err == nil && ok {
		payload["managed_run"] = true
		payload["run_branch"] = strings.TrimSpace(run.Branch)
		payload["run_worktree"] = strings.TrimSpace(run.WorktreePath)
	}

	err := lifecycle.Append(repo, lifecycle.Event{
		Version: lifecycle.SchemaVersionV1,
		Type:    lifecycle.EventStateTransition,
		Payload: payload,
	})
	if err != nil && out != nil {
		_, _ = io.WriteString(out, "docket: warning: state transition event emit failed: "+err.Error()+"\n")
	}
}

func loadTicketTransitionHistory(repoRoot, ticketID string) ([]transitionHistoryEntry, error) {
	events, err := lifecycle.Load(repoRoot)
	if err != nil {
		return nil, err
	}
	history := make([]transitionHistoryEntry, 0, len(events))
	for _, ev := range events {
		if ev.Type != lifecycle.EventStateTransition {
			continue
		}
		id, _ := ev.Payload["ticket_id"].(string)
		if strings.TrimSpace(id) != ticketID {
			continue
		}
		entry := transitionHistoryEntry{
			At:          coerceTime(ev.EmittedAt),
			Command:     coerceString(ev.Payload["command"]),
			Actor:       coerceString(ev.Payload["actor"]),
			FromState:   coerceString(ev.Payload["from_state"]),
			ToState:     coerceString(ev.Payload["to_state"]),
			Reason:      coerceString(ev.Payload["reason"]),
			Checks:      coerceStringSlice(ev.Payload["checks"]),
			ManagedRun:  coerceBool(ev.Payload["managed_run"]),
			RunBranch:   coerceString(ev.Payload["run_branch"]),
			RunWorktree: coerceString(ev.Payload["run_worktree"]),
		}
		history = append(history, entry)
	}
	sort.SliceStable(history, func(i, j int) bool {
		return history[i].At < history[j].At
	})
	return history, nil
}

func normalizeChecks(checks []string) []string {
	if len(checks) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		check = strings.TrimSpace(check)
		if check == "" {
			continue
		}
		if _, ok := seen[check]; ok {
			continue
		}
		seen[check] = struct{}{}
		out = append(out, check)
	}
	sort.Strings(out)
	return out
}

func coerceString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func coerceBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func coerceStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(s))
	}
	sort.Strings(out)
	return out
}

func coerceTime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return v
}
