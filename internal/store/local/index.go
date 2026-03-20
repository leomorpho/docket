package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/artifacts"
	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

func (s *Store) IndexPath() string {
	return artifacts.WriteRepoPath(s.RepoRoot, artifacts.RepoIndexDB)
}

func (s *Store) isIndexStale() bool {
	dbInfo, err := os.Stat(s.IndexPath())
	if err != nil {
		return true // Missing index is stale
	}

	ticketsDir := filepath.Join(s.RepoRoot, ".docket", "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			info, err := entry.Info()
			if err == nil && !info.ModTime().Before(dbInfo.ModTime()) {
				return true
			}
		}
	}
	return false
}

func (s *Store) ensureIndex(ctx context.Context) error {
	s.indexSyncMu.Lock()
	defer s.indexSyncMu.Unlock()
	if s.indexFresh && !s.isIndexStale() {
		return nil
	}
	return s.syncIndexWithRetry(ctx)
}

func (s *Store) SyncIndex(ctx context.Context) error {
	s.indexSyncMu.Lock()
	defer s.indexSyncMu.Unlock()
	s.indexFresh = false
	return s.syncIndexWithRetry(ctx)
}

func (s *Store) syncIndexWithRetry(ctx context.Context) error {
	err := withSQLiteBusyRetry(ctx, "sync index", func() error {
		return s.syncIndexOnce(ctx)
	})
	if err == nil {
		s.indexFresh = true
	}
	return err
}

func (s *Store) syncIndexOnce(ctx context.Context) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		DROP TABLE IF EXISTS labels;
		DROP TABLE IF EXISTS blocked_by;
		DROP TABLE IF EXISTS linked_commits;
		DROP TABLE IF EXISTS tickets;

		CREATE TABLE tickets (
			id            TEXT PRIMARY KEY,
			seq           INTEGER NOT NULL,
			state         TEXT NOT NULL,
			priority      INTEGER NOT NULL DEFAULT 10,
			parent        TEXT,
			title         TEXT NOT NULL,
			created_by    TEXT NOT NULL,
			created_at    DATETIME NOT NULL,
			updated_at    DATETIME NOT NULL,
			is_blocked    INTEGER NOT NULL DEFAULT 0,
			ac_total      INTEGER NOT NULL DEFAULT 0,
			ac_done       INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE labels (
			ticket_id  TEXT NOT NULL REFERENCES tickets(id),
			label      TEXT NOT NULL
		);

		CREATE TABLE blocked_by (
			ticket_id  TEXT NOT NULL REFERENCES tickets(id),
			blocks_id  TEXT NOT NULL
		);

		CREATE TABLE linked_commits (
			ticket_id  TEXT NOT NULL REFERENCES tickets(id),
			sha        TEXT NOT NULL
		);

		CREATE INDEX idx_tickets_state ON tickets(state);
		CREATE INDEX idx_tickets_priority ON tickets(priority);
		CREATE INDEX idx_tickets_parent ON tickets(parent);
		CREATE INDEX idx_labels_ticket ON labels(ticket_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Re-parse all markdown files and insert
	ticketsDir := filepath.Join(s.RepoRoot, ".docket", "tickets")
	entries, err := os.ReadDir(ticketsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	type ticketRow struct {
		t       *ticket.Ticket
		acTotal int
		acDone  int
	}
	rowsToInsert := make([]ticketRow, 0, len(entries))
	ticketsByID := make(map[string]*ticket.Ticket, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			id := entry.Name()[:len(entry.Name())-3]
			if id == "" {
				continue
			}
			t, err := s.GetTicket(ctx, id)
			if err != nil {
				continue
			}

			acTotal := len(t.AC)
			acDone := 0
			for _, ac := range t.AC {
				if ac.Done {
					acDone++
				}
			}
			rowsToInsert = append(rowsToInsert, ticketRow{
				t:       t,
				acTotal: acTotal,
				acDone:  acDone,
			})
			ticketsByID[t.ID] = t
		}
	}
	cfg := s.loadConfigOrDefault()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmtTicket, err := tx.PrepareContext(ctx, `INSERT INTO tickets (id, seq, state, priority, parent, title, created_by, created_at, updated_at, is_blocked, ac_total, ac_done) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmtTicket.Close()
	stmtLabel, err := tx.PrepareContext(ctx, `INSERT INTO labels (ticket_id, label) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmtLabel.Close()
	stmtBlocked, err := tx.PrepareContext(ctx, `INSERT INTO blocked_by (ticket_id, blocks_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmtBlocked.Close()
	stmtCommit, err := tx.PrepareContext(ctx, `INSERT INTO linked_commits (ticket_id, sha) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmtCommit.Close()

	for _, row := range rowsToInsert {
		isBlocked := false
		for _, blockerID := range row.t.BlockedBy {
			blocker, ok := ticketsByID[blockerID]
			if !ok || cfg.BlocksDependents(blocker.State) {
				isBlocked = true
				break
			}
		}
		_, err = stmtTicket.ExecContext(ctx, row.t.ID, row.t.Seq, row.t.State, row.t.Priority, row.t.Parent, row.t.Title, row.t.CreatedBy, row.t.CreatedAt, row.t.UpdatedAt, isBlocked, row.acTotal, row.acDone)
		if err != nil {
			return err
		}

		for _, l := range row.t.Labels {
			if _, err := stmtLabel.ExecContext(ctx, row.t.ID, l); err != nil {
				return err
			}
		}
		for _, b := range row.t.BlockedBy {
			if _, err := stmtBlocked.ExecContext(ctx, row.t.ID, b); err != nil {
				return err
			}
		}
		for _, c := range row.t.LinkedCommits {
			if _, err := stmtCommit.ExecContext(ctx, row.t.ID, c); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *Store) queryTickets(ctx context.Context, f store.Filter) ([]*ticket.Ticket, error) {
	if err := s.ensureIndex(ctx); err != nil {
		return nil, err
	}

	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := "SELECT id, seq, state, priority, parent, title, created_by, created_at, updated_at FROM tickets WHERE 1=1"
	var args []interface{}

	if !f.IncludeArchived {
		query += " AND state != ?"
		args = append(args, "archived")
	}

	if len(f.States) > 0 {
		placeholders := make([]string, len(f.States))
		for i, st := range f.States {
			placeholders[i] = "?"
			args = append(args, st)
		}
		query += fmt.Sprintf(" AND state IN (%s)", strings.Join(placeholders, ","))
	}

	if f.MaxPriority > 0 {
		query += " AND priority <= ?"
		args = append(args, f.MaxPriority)
	}

	if len(f.Labels) > 0 {
		for _, l := range f.Labels {
			query += " AND id IN (SELECT ticket_id FROM labels WHERE label = ?)"
			args = append(args, l)
		}
	}

	query += " ORDER BY priority ASC, created_at ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ticket.Ticket
	for rows.Next() {
		t := &ticket.Ticket{}
		var createdAt, updatedAt string
		err := rows.Scan(&t.ID, &t.Seq, &t.State, &t.Priority, &t.Parent, &t.Title, &t.CreatedBy, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		results = append(results, t)
	}

	if f.OnlyUnblocked {
		cfg := s.loadConfigOrDefault()
		filtered := results[:0]
		for _, t := range results {
			full, err := s.GetTicket(ctx, t.ID)
			if err != nil {
				return nil, err
			}
			if full == nil {
				continue
			}
			if !s.hasUnresolvedBlockers(ctx, full, cfg) {
				filtered = append(filtered, t)
			}
		}
		results = filtered
	}

	return results, nil
}
