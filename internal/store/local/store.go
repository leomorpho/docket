package local

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	docketgit "github.com/leomorpho/docket/internal/git"
	"github.com/leomorpho/docket/internal/proof"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
	_ "modernc.org/sqlite"
)

type Store struct {
	RepoRoot string

	mu     sync.RWMutex
	relIdx *RelationshipIndex

	indexSyncMu sync.Mutex
	indexFresh  bool

	annotationSchemaOnce sync.Once
	annotationSchemaErr  error
	annotationMu         sync.Mutex
}

var (
	storesMu sync.Mutex
	stores   = make(map[string]*Store)
)

func New(repoRoot string) *Store {
	absRepo := docketgit.SharedRepoRoot(repoRoot)

	storesMu.Lock()
	defer storesMu.Unlock()

	if s, ok := stores[absRepo]; ok {
		return s
	}

	s := &Store{RepoRoot: absRepo}
	stores[absRepo] = s
	return s
}

func (s *Store) openDB() (*sql.DB, error) {
	dbPath := s.IndexPath()
	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	// Add busy timeout and WAL mode to handle concurrent access
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL", dbPath)
	return sql.Open("sqlite", dsn)
}

func (s *Store) ticketPath(id string) string {
	id = s.normalizeTicketLookupID(id)
	return filepath.Join(s.RepoRoot, ".docket", "tickets", id+".md")
}

func (s *Store) normalizeTicketLookupID(id string) string {
	if strings.HasSuffix(id, ".md") {
		id = strings.TrimSuffix(filepath.Base(id), ".md")
	}
	if normalized, ok := ticket.NormalizeID(id); ok {
		return normalized
	}
	return id
}

func (s *Store) CreateTicket(ctx context.Context, t *ticket.Ticket) error {
	if t.ID == "" {
		return fmt.Errorf("ticket ID is required")
	}
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

	if err := signTicket(t); err != nil {
		return err
	}
	content, err := render(t)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	s.InvalidateRelationshipIndex()
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

	// Preserve lifecycle timestamps by default, and stamp them on forward transitions.
	if t.StartedAt.IsZero() {
		t.StartedAt = existing.StartedAt
	}
	if t.CompletedAt.IsZero() {
		t.CompletedAt = existing.CompletedAt
	}
	if existing.State != t.State {
		now := time.Now().UTC().Truncate(time.Second)
		cfg := s.loadConfigOrDefault()
		if cfg.StateHasRole(string(t.State), "active") && t.StartedAt.IsZero() {
			t.StartedAt = now
		}
		if cfg.StateHasRole(string(t.State), "completed") {
			if t.StartedAt.IsZero() {
				t.StartedAt = now
			}
			if t.CompletedAt.IsZero() {
				t.CompletedAt = now
			}
		}
	}
	if err := s.validateParentRef(ctx, t); err != nil {
		return err
	}

	// If the provided ticket has no comments, use the existing ones
	if len(t.Comments) == 0 {
		t.Comments = existing.Comments
	}

	if err := signTicket(t); err != nil {
		return err
	}
	content, err := render(t)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	s.InvalidateRelationshipIndex()
	return s.upsertManifestTicket(t.ID, s.manifestEntryFromTicket(t))
}

func (s *Store) GetTicket(ctx context.Context, id string) (*ticket.Ticket, error) {
	data, err := s.GetRaw(ctx, id)
	if err != nil || data == "" {
		return nil, err
	}

	return Parse(data)
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

func (s *Store) loadConfigOrDefault() *ticket.Config {
	cfg, err := ticket.LoadConfig(s.RepoRoot)
	if err != nil {
		return ticket.DefaultConfig()
	}
	return cfg
}

func (s *Store) hasUnresolvedBlockers(ctx context.Context, t *ticket.Ticket, cfg *ticket.Config) bool {
	blockers, _ := s.unresolvedBlockers(ctx, t, cfg)
	return len(blockers) > 0
}

func (s *Store) unresolvedBlockers(ctx context.Context, t *ticket.Ticket, cfg *ticket.Config) ([]string, error) {
	var unresolved []string
	for _, blockerID := range t.BlockedBy {
		blocker, err := s.GetTicket(ctx, blockerID)
		if err != nil {
			return nil, err
		}
		if blocker == nil || cfg.BlocksDependents(blocker.State) {
			unresolved = append(unresolved, blockerID)
		}
	}
	return unresolved, nil
}

func (s *Store) UnresolvedBlockers(ctx context.Context, t *ticket.Ticket) ([]string, error) {
	return s.unresolvedBlockers(ctx, t, s.loadConfigOrDefault())
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

	if f.OnlyUnblocked && len(t.BlockedBy) > 0 {
		cfg := s.loadConfigOrDefault()
		if s.hasUnresolvedBlockers(context.Background(), t, cfg) {
			return false
		}
	}

	return true
}

func (s *Store) DetectCycleValidationError() *store.ValidationError {
	if cycleErr := s.detectCycles(); cycleErr != nil {
		return &store.ValidationError{Field: "dependencies", Message: cycleErr.Error()}
	}
	return nil
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

func (s *Store) AddProof(ctx context.Context, in proof.AddInput) (*proof.Record, error) {
	t, err := s.GetTicket(ctx, in.TicketID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", in.TicketID)
	}
	in.TicketID = t.ID
	repo := proof.NewRepository(s.RepoRoot)
	return repo.Add(ctx, in)
}

func (s *Store) ListProofs(ctx context.Context, ticketID string) ([]proof.Record, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}
	repo := proof.NewRepository(s.RepoRoot)
	return repo.List(ctx, t.ID)
}

func (s *Store) RemoveProof(ctx context.Context, ticketID string, proofID string) (*proof.Record, error) {
	t, err := s.GetTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}
	repo := proof.NewRepository(s.RepoRoot)
	return repo.Remove(ctx, t.ID, proofID)
}

func (s *Store) GCProofBlobs(ctx context.Context) (proof.GCSummary, error) {
	repo := proof.NewRepository(s.RepoRoot)
	return repo.GC(ctx)
}

func (s *Store) NextID(ctx context.Context) (id string, seq int, err error) {
	return ticket.NextID(s.RepoRoot)
}
