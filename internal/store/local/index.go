package local

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
	_ "modernc.org/sqlite"
)

func (s *Store) IndexPath() string {
	return filepath.Join(s.RepoRoot, ".docket", "index.db")
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
			if err == nil && info.ModTime().After(dbInfo.ModTime()) {
				return true
			}
		}
	}
	return false
}

func (s *Store) ensureIndex(ctx context.Context) error {
	if s.isIndexStale() {
		return s.SyncIndex(ctx)
	}
	return nil
}

func (s *Store) SyncIndex(ctx context.Context) error {
	dbPath := s.IndexPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		DROP TABLE IF EXISTS labels;
		DROP TABLE IF EXISTS blocked_by;
		DROP TABLE IF EXISTS linked_commits;
		DROP TABLE IF EXISTS annotations;
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

		CREATE TABLE annotations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ticket_id  TEXT NOT NULL,
			file_path  TEXT NOT NULL,
			line_num   INTEGER NOT NULL,
			context    TEXT NOT NULL
		);

		CREATE INDEX idx_tickets_state ON tickets(state);
		CREATE INDEX idx_tickets_priority ON tickets(priority);
		CREATE INDEX idx_tickets_parent ON tickets(parent);
		CREATE INDEX idx_labels_ticket ON labels(ticket_id);
		CREATE INDEX idx_annotations_ticket ON annotations(ticket_id);
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

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmtTicket, _ := tx.PrepareContext(ctx, `INSERT INTO tickets (id, seq, state, priority, parent, title, created_by, created_at, updated_at, is_blocked, ac_total, ac_done) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	stmtLabel, _ := tx.PrepareContext(ctx, `INSERT INTO labels (ticket_id, label) VALUES (?, ?)`)
	stmtBlocked, _ := tx.PrepareContext(ctx, `INSERT INTO blocked_by (ticket_id, blocks_id) VALUES (?, ?)`)
	stmtCommit, _ := tx.PrepareContext(ctx, `INSERT INTO linked_commits (ticket_id, sha) VALUES (?, ?)`)

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			id := entry.Name()[:len(entry.Name())-3]
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

			_, err = stmtTicket.ExecContext(ctx, t.ID, t.Seq, t.State, t.Priority, t.Parent, t.Title, t.CreatedBy, t.CreatedAt, t.UpdatedAt, t.IsBlocked(), acTotal, acDone)
			if err != nil {
				return err
			}

			for _, l := range t.Labels {
				stmtLabel.ExecContext(ctx, t.ID, l)
			}
			for _, b := range t.BlockedBy {
				stmtBlocked.ExecContext(ctx, t.ID, b)
			}
			for _, c := range t.LinkedCommits {
				stmtCommit.ExecContext(ctx, t.ID, c)
			}
			count++
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

	db, err := sql.Open("sqlite", s.IndexPath())
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

	if f.OnlyUnblocked {
		query += " AND is_blocked = 0"
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

	return results, nil
}
