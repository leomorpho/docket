package local

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
