package local

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestSQLiteConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	s := New(tmpDir)
	ctx := context.Background()

	const numWorkers = 5
	const iterations = 10
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	errChan := make(chan error, numWorkers*iterations)

	// Run multiple workers that all try to write/read concurrently
	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				ann := []Annotation{
					{TicketID: "TKT-001", FilePath: "file.go", LineNum: i, Context: fmt.Sprintf("worker %d", workerID)},
				}
				if err := s.UpsertAnnotations(ctx, ann); err != nil {
					errChan <- fmt.Errorf("worker %d iteration %d write: %w", workerID, i, err)
				}
				
				_, err := s.GetAnnotationsByTicket(ctx, "TKT-001")
				if err != nil {
					errChan <- fmt.Errorf("worker %d iteration %d read: %w", workerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrency error: %v", err)
	}
}
