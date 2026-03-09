package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	content, err := render(t)
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0644)
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

	// If the provided ticket has no comments, use the existing ones
	if len(t.Comments) == 0 {
		t.Comments = existing.Comments
	}

	content, err := render(t)
	if err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0644)
}

func (s *Store) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	path := s.ticketPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	return parse(string(data))
}

func (s *Store) ListTickets(ctx context.Context, f store.Filter) ([]*ticket.Ticket, error) {
	ticketsDir := filepath.Join(s.RepoRoot, ".docket", "tickets")
	files, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ticket.Ticket{}, nil
		}
		return nil, err
	}

	var results []*ticket.Ticket
	for _, entry := range files {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			id := strings.TrimSuffix(entry.Name(), ".md")
			t, err := s.GetTicket(ctx, id)
			if err != nil {
				continue // Skip corrupt tickets for now
			}

			if s.matches(t, f) {
				results = append(results, t)
			}
		}
	}

	// Sort by priority ascending (1 is highest), then CreatedAt ascending
	sort.Slice(results, func(i, j int) bool {
		if results[i].Priority != results[j].Priority {
			return results[i].Priority < results[j].Priority
		}
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})

	return results, nil
}

func (s *Store) matches(t *ticket.Ticket, f store.Filter) bool {
	if !f.IncludeArchived && t.State == ticket.StateArchived {
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
		if t.State == ticket.StateArchived {
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
	return s.UpdateTicket(ctx, t)
}

func (s *Store) NextID(ctx context.Context) (id string, seq int, err error) {
	return ticket.NextID(s.RepoRoot)
}

func (s *Store) Validate(ctx context.Context, id string) ([]store.ValidationError, error) {
	return nil, nil // Stub for TASK-005
}
