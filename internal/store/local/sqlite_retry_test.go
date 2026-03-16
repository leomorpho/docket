package local

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leomorpho/docket/internal/ticket"
)

func TestWithSQLiteBusyRetryRetriesAndSucceeds(t *testing.T) {
	prevRetries := sqliteBusyMaxRetries
	prevDelay := sqliteBusyBaseDelay
	prevSleep := sqliteBusySleep
	defer func() {
		sqliteBusyMaxRetries = prevRetries
		sqliteBusyBaseDelay = prevDelay
		sqliteBusySleep = prevSleep
	}()

	sqliteBusyMaxRetries = 3
	sqliteBusyBaseDelay = 0
	sqliteBusySleep = func(time.Duration) {}

	attempts := 0
	err := withSQLiteBusyRetry(context.Background(), "unit op", func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithSQLiteBusyRetryBoundedFailure(t *testing.T) {
	prevRetries := sqliteBusyMaxRetries
	prevDelay := sqliteBusyBaseDelay
	prevSleep := sqliteBusySleep
	defer func() {
		sqliteBusyMaxRetries = prevRetries
		sqliteBusyBaseDelay = prevDelay
		sqliteBusySleep = prevSleep
	}()

	sqliteBusyMaxRetries = 2
	sqliteBusyBaseDelay = 0
	sqliteBusySleep = func(time.Duration) {}

	attempts := 0
	err := withSQLiteBusyRetry(context.Background(), "bounded op", func() error {
		attempts++
		return fmt.Errorf("database is locked (5) (SQLITE_BUSY)")
	})
	if err == nil {
		t.Fatal("expected bounded failure")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts (initial + retries), got %d", attempts)
	}
	if !strings.Contains(err.Error(), "failed after 3 attempts") {
		t.Fatalf("expected bounded retry details, got: %v", err)
	}
}

func TestWithSQLiteBusyRetryNonRetryable(t *testing.T) {
	attempts := 0
	err := withSQLiteBusyRetry(context.Background(), "non-retryable", func() error {
		attempts++
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected non-retryable failure")
	}
	if attempts != 1 {
		t.Fatalf("expected no retries for non-retryable errors, got %d attempts", attempts)
	}
}

func TestSQLiteConcurrentMutationPressureReducedBusyFailures(t *testing.T) {
	baselineBusy, baselineOther := runMutationPressureScenario(t, 0)
	hardenedBusy, hardenedOther := runMutationPressureScenario(t, defaultSQLiteBusyRetries)
	t.Logf("mutation pressure busy-error rates: baseline_no_retry=%d hardened_retry=%d", baselineBusy, hardenedBusy)

	if baselineBusy == 0 {
		t.Fatalf("expected baseline run without retries to surface SQLITE_BUSY errors, got %d", baselineBusy)
	}
	if hardenedBusy >= baselineBusy {
		t.Fatalf("expected hardened retry policy to reduce SQLITE_BUSY errors (baseline=%d hardened=%d)", baselineBusy, hardenedBusy)
	}
	if baselineOther != 0 || hardenedOther != 0 {
		t.Fatalf("expected no non-busy errors (baseline=%d hardened=%d)", baselineOther, hardenedOther)
	}
}

func runMutationPressureScenario(t *testing.T, retries int) (busyErrs int, otherErrs int) {
	t.Helper()
	prevRetries := sqliteBusyMaxRetries
	sqliteBusyMaxRetries = retries
	defer func() { sqliteBusyMaxRetries = prevRetries }()

	tmpDir := t.TempDir()
	if err := ticket.SaveConfig(tmpDir, ticket.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	s := New(tmpDir)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("TKT-%03d", i)
		tkt := &ticket.Ticket{
			ID:          id,
			Seq:         i,
			State:       ticket.State("backlog"),
			Priority:    1,
			Title:       fmt.Sprintf("Ticket %d", i),
			Description: "Concurrency pressure fixture ticket.",
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "agent:test",
			AC:          []ticket.AcceptanceCriterion{{Description: "ok"}},
		}
		if err := s.CreateTicket(ctx, tkt); err != nil {
			t.Fatalf("create fixture ticket %s: %v", id, err)
		}
	}

	const workers = 6
	const iterations = 10
	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := s.SyncIndex(ctx); err != nil {
					errCh <- fmt.Errorf("worker %d sync iteration %d: %w", worker, i, err)
					continue
				}
				anns := []Annotation{{TicketID: "TKT-001", FilePath: "file.go", LineNum: i, Context: fmt.Sprintf("worker-%d", worker)}}
				if err := s.UpsertAnnotations(ctx, anns); err != nil {
					errCh <- fmt.Errorf("worker %d upsert iteration %d: %w", worker, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	busyErrs = 0
	otherErrs = 0
	for err := range errCh {
		if isRetryableSQLiteBusy(err) {
			busyErrs++
			continue
		}
		otherErrs++
		t.Logf("non-busy mutation error: %v", err)
	}
	return busyErrs, otherErrs
}
