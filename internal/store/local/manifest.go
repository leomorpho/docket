package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

type ManifestTicket struct {
	Title    string `json:"title"`
	State    string `json:"state"`
	Priority int    `json:"priority"`
	Parent   string `json:"parent,omitempty"`
}

type Manifest struct {
	Warning string                    `json:"_warning"`
	Tickets map[string]ManifestTicket `json:"tickets"`
}

type TamperChange struct {
	ID       string
	Field    string
	Expected string
	Actual   string
}

func (s *Store) manifestPath() string {
	return filepath.Join(s.RepoRoot, ".docket", "manifest.json")
}

func defaultManifest() Manifest {
	return Manifest{
		Warning: "DO NOT EDIT .docket/tickets/*.md OR .docket/manifest.json DIRECTLY. Use `docket` commands only.",
		Tickets: map[string]ManifestTicket{},
	}
}

func (s *Store) loadManifest() (Manifest, error) {
	path := s.manifestPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultManifest(), nil
		}
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if m.Warning == "" {
		m.Warning = defaultManifest().Warning
	}
	if m.Tickets == nil {
		m.Tickets = map[string]ManifestTicket{}
	}
	return m, nil
}

func (s *Store) saveManifest(m Manifest) error {
	path := s.manifestPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func (s *Store) ensureManifest(ctx context.Context) error {
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	if len(m.Tickets) > 0 {
		return nil
	}
	all, err := s.ListTickets(ctx, store.Filter{IncludeArchived: true})
	if err != nil {
		return err
	}
	for _, t := range all {
		m.Tickets[t.ID] = ManifestTicket{
			Title:    t.Title,
			State:    string(t.State),
			Priority: t.Priority,
			Parent:   t.Parent,
		}
	}
	return s.saveManifest(m)
}

func (s *Store) upsertManifestTicket(tid string, entry ManifestTicket) error {
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	m.Tickets[tid] = entry
	return s.saveManifest(m)
}

func (s *Store) removeManifestTicket(tid string) error {
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	delete(m.Tickets, tid)
	return s.saveManifest(m)
}

func (s *Store) manifestEntryFromTicket(t *ticket.Ticket) ManifestTicket {
	return ManifestTicket{
		Title:    t.Title,
		State:    string(t.State),
		Priority: t.Priority,
		Parent:   t.Parent,
	}
}

func (s *Store) DetectTampering(ctx context.Context, id string) ([]TamperChange, error) {
	if err := s.ensureManifest(ctx); err != nil {
		return nil, err
	}
	m, err := s.loadManifest()
	if err != nil {
		return nil, err
	}
	t, err := s.GetTicket(ctx, id)
	if err != nil || t == nil {
		return nil, err
	}
	entry, ok := m.Tickets[id]
	if !ok {
		return nil, nil
	}
	return diffManifestTicket(id, entry, s.manifestEntryFromTicket(t)), nil
}

func (s *Store) DetectTamperingAll(ctx context.Context) ([]TamperChange, error) {
	if err := s.ensureManifest(ctx); err != nil {
		return nil, err
	}
	m, err := s.loadManifest()
	if err != nil {
		return nil, err
	}
	all, err := s.ListTickets(ctx, store.Filter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	var out []TamperChange
	for _, t := range all {
		entry, ok := m.Tickets[t.ID]
		if !ok {
			continue
		}
		out = append(out, diffManifestTicket(t.ID, entry, s.manifestEntryFromTicket(t))...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == out[j].ID {
			return out[i].Field < out[j].Field
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ReconcileResult holds the outcome of reconciling a directly-edited ticket.
type ReconcileResult struct {
	ID       string
	Accepted bool
	Changes  []TamperChange
	Errors   []store.ValidationError
}

// ReconcileTampering detects direct edits and accepts valid ones, rejects invalid ones.
// Accepted tickets have their manifest entries updated to reflect the new values.
// Rejected tickets are left unchanged in the manifest.
func (s *Store) ReconcileTampering(ctx context.Context) ([]ReconcileResult, error) {
	changes, err := s.DetectTamperingAll(ctx)
	if err != nil {
		return nil, err
	}

	// Group changes by ticket ID
	byID := make(map[string][]TamperChange)
	for _, ch := range changes {
		byID[ch.ID] = append(byID[ch.ID], ch)
	}

	var results []ReconcileResult
	for id, ticketChanges := range byID {
		schemaErrs, _, err := s.ValidateFile(id)
		if err != nil {
			// Can't validate — skip this ticket
			continue
		}

		result := ReconcileResult{
			ID:      id,
			Changes: ticketChanges,
			Errors:  schemaErrs,
		}

		if len(schemaErrs) == 0 {
			// Schema-valid: accept the direct edit by updating the manifest
			t, err := s.GetTicket(ctx, id)
			if err == nil && t != nil {
				entry := s.manifestEntryFromTicket(t)
				if upsertErr := s.upsertManifestTicket(id, entry); upsertErr == nil {
					result.Accepted = true
				}
			}
		}
		// If schema errors exist, leave Accepted=false and include errors

		results = append(results, result)
	}

	// Sort results by ID for deterministic output
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	return results, nil
}

func diffManifestTicket(id string, expected, actual ManifestTicket) []TamperChange {
	var out []TamperChange
	if expected.Title != actual.Title {
		out = append(out, TamperChange{ID: id, Field: "title", Expected: expected.Title, Actual: actual.Title})
	}
	if expected.State != actual.State {
		out = append(out, TamperChange{ID: id, Field: "state", Expected: expected.State, Actual: actual.State})
	}
	if expected.Priority != actual.Priority {
		out = append(out, TamperChange{ID: id, Field: "priority", Expected: itoa(expected.Priority), Actual: itoa(actual.Priority)})
	}
	if expected.Parent != actual.Parent {
		out = append(out, TamperChange{ID: id, Field: "parent", Expected: expected.Parent, Actual: actual.Parent})
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
