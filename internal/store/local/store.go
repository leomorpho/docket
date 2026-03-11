package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leoaudibert/docket/internal/store"
	"github.com/leoaudibert/docket/internal/ticket"
)

type Store struct {
	RepoRoot string
}

func New(repoRoot string) *Store {
	return &Store{RepoRoot: repoRoot}
}

func (s *Store) ticketPath(id string) string {
	return filepath.Join(s.RepoRoot, ".docket", "tickets", id+".md")
}

func (s *Store) CreateTicket(ctx context.Context, t *ticket.Ticket) error {
	path := s.ticketPath(t.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("ticket %s already exists", t.ID)
	}
	if err := s.validateParentRef(ctx, t); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	content, err := render(t)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return s.upsertManifestTicket(t.ID, s.manifestEntryFromTicket(t))
}

func (s *Store) UpdateTicket(ctx context.Context, t *ticket.Ticket) error {
	path := s.ticketPath(t.ID)
	// Preserve existing comments if they are not in the ticket struct (though they should be)
	// But according to TASK-004, we parse existing file to get comments then rewrite.
	existing, err := s.GetTicket(ctx, t.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("ticket %s not found", t.ID)
	}
	if err := s.validateParentRef(ctx, t); err != nil {
		return err
	}

	// If the provided ticket has no comments, use the existing ones
	if len(t.Comments) == 0 {
		t.Comments = existing.Comments
	}

	content, err := render(t)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return s.upsertManifestTicket(t.ID, s.manifestEntryFromTicket(t))
}

func (s *Store) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	data, err := s.GetRaw(ctx, id)
	if err != nil || data == "" {
		return nil, err
	}

	return parse(data)
}

func (s *Store) GetRaw(ctx context.Context, id string) (string, error) {
	path := s.ticketPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (s *Store) ListTickets(ctx context.Context, f store.Filter) ([]*ticket.Ticket, error) {
	tickets, err := s.queryTickets(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("querying tickets from index: %w", err)
	}

	// For any listed ticket, we might want to ensure labels/blocked_by are fully populated
	// if they aren't in the base SELECT. But for List view, the basic fields are enough.
	// We'll leave them as-is from the Scan.
	return tickets, nil
}

func (s *Store) matches(t *ticket.Ticket, f store.Filter) bool {
	if !f.IncludeArchived && t.State == "archived" {
		return false
	}

	if len(f.States) > 0 {
		found := false
		for _, state := range f.States {
			if t.State == state {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	} else if !f.IncludeArchived {
		// Default: all non-archived
		if t.State == "archived" {
			return false
		}
	}

	if len(f.Labels) > 0 {
		for _, required := range f.Labels {
			found := false
			for _, actual := range t.Labels {
				if actual == required {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	if f.MaxPriority > 0 && t.Priority > f.MaxPriority {
		return false
	}

	if f.OnlyUnblocked && t.IsBlocked() {
		return false
	}

	return true
}

func (s *Store) AddComment(ctx context.Context, id string, c ticket.Comment) error {
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("ticket %s not found", id)
	}

	t.Comments = append(t.Comments, c)
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	return s.UpdateTicket(ctx, t)
}

func (s *Store) LinkCommit(ctx context.Context, id string, sha string) error {
	t, err := s.GetTicket(ctx, id)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("ticket %s not found", id)
	}

	for _, existing := range t.LinkedCommits {
		if existing == sha {
			return nil
		}
	}
	t.LinkedCommits = append(t.LinkedCommits, sha)
	t.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	return s.UpdateTicket(ctx, t)
}

func (s *Store) NextID(ctx context.Context) (id string, seq int, err error) {
	return ticket.NextID(s.RepoRoot)
}
