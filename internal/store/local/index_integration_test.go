package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

func TestSyncIndexConcurrentCallsDoNotSurfaceBusy(t *testing.T) {
	prevRetries := sqliteBusyMaxRetries
	sqliteBusyMaxRetries = 0
	defer func() { sqliteBusyMaxRetries = prevRetries }()

	tmpDir := t.TempDir()
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("TKT-%03d", i)
		if err := s.CreateTicket(ctx, &ticket.Ticket{
			ID:          id,
			Seq:         i,
			State:       ticket.State("todo"),
			Priority:    1,
			Title:       "fixture " + id,
			Description: "fixture ticket for sync-index lock contention integration test",
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			AC:          []ticket.AcceptanceCriterion{{Description: "ok"}},
		}); err != nil {
			t.Fatalf("create fixture ticket %s: %v", id, err)
		}
	}

	const workers = 6
	const iterations = 6
	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := s.SyncIndex(ctx); err != nil {
					errCh <- fmt.Errorf("worker %d iteration %d sync: %w", worker, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if isRetryableSQLiteBusy(err) || strings.Contains(strings.ToLower(err.Error()), "sqlite_busy") {
			t.Fatalf("expected no SQLITE_BUSY surfaced from concurrent sync calls, got: %v", err)
		}
		t.Fatalf("unexpected sync failure: %v", err)
	}
}

func TestSyncIndexPreservesAnnotations(t *testing.T) {
	tmpDir := t.TempDir()
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		State:       ticket.State("todo"),
		Priority:    1,
		Title:       "fixture TKT-001",
		Description: "fixture ticket for annotation preservation integration test",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		AC:          []ticket.AcceptanceCriterion{{Description: "ok"}},
	}); err != nil {
		t.Fatalf("create fixture ticket: %v", err)
	}

	seed := []Annotation{
		{TicketID: "TKT-001", FilePath: "main.go", LineNum: 7, Context: "// [TKT-001] keep me"},
	}
	if err := s.UpsertAnnotations(ctx, seed); err != nil {
		t.Fatalf("upsert annotations: %v", err)
	}

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("sync index: %v", err)
	}

	got, err := s.GetAnnotationsByTicket(ctx, "TKT-001")
	if err != nil {
		t.Fatalf("get annotations after sync: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 annotation after sync, got %d: %#v", len(got), got)
	}
	if got[0].FilePath != seed[0].FilePath || got[0].LineNum != seed[0].LineNum || got[0].Context != seed[0].Context {
		t.Fatalf("annotation changed across sync: got=%#v want=%#v", got[0], seed[0])
	}
}

func TestListTicketsAutoRefreshesIndexAfterTicketFileChanges(t *testing.T) {
	tmpDir := t.TempDir()
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.CreateTicket(ctx, &ticket.Ticket{
		ID:          "TKT-001",
		Seq:         1,
		State:       ticket.State("backlog"),
		Priority:    1,
		Title:       "fixture TKT-001",
		Description: "fixture ticket for auto-refresh integration test",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "agent:test",
		AC:          []ticket.AcceptanceCriterion{{Description: "ok"}},
	}); err != nil {
		t.Fatalf("create fixture ticket: %v", err)
	}

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("sync index: %v", err)
	}

	path := filepath.Join(tmpDir, ".docket", "tickets", "TKT-001.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket file: %v", err)
	}
	updated := strings.Replace(string(data), "state: backlog", "state: todo", 1)
	if updated == string(data) {
		t.Fatalf("expected to rewrite state in ticket file")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write ticket file: %v", err)
	}

	tickets, err := s.ListTickets(ctx, store.Filter{States: []ticket.State{ticket.State("todo")}})
	if err != nil {
		t.Fatalf("ListTickets() error = %v", err)
	}
	if len(tickets) != 1 || tickets[0].ID != "TKT-001" || tickets[0].State != ticket.State("todo") {
		t.Fatalf("expected refreshed todo ticket after file edit, got %#v", tickets)
	}
}

func TestListTicketsAfterExplicitSyncDoesNotResyncAndDuplicateRows(t *testing.T) {
	tmpDir := t.TempDir()
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("TKT-%03d", i)
		if err := s.CreateTicket(ctx, &ticket.Ticket{
			ID:          id,
			Seq:         i,
			State:       ticket.State("backlog"),
			Priority:    i,
			Title:       "fixture " + id,
			Description: "fixture ticket for double-sync integration test",
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			AC:          []ticket.AcceptanceCriterion{{Description: "ok"}},
		}); err != nil {
			t.Fatalf("create fixture ticket %s: %v", id, err)
		}
	}

	if err := s.SyncIndex(ctx); err != nil {
		t.Fatalf("SyncIndex() error = %v", err)
	}

	tickets, err := s.ListTickets(ctx, store.Filter{States: []ticket.State{ticket.State("backlog")}})
	if err != nil {
		t.Fatalf("ListTickets() after explicit SyncIndex error = %v", err)
	}
	if len(tickets) != 3 {
		t.Fatalf("expected 3 tickets after explicit sync, got %d: %#v", len(tickets), tickets)
	}
}
