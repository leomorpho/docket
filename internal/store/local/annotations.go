package local

import (
	"context"
	"database/sql"
	"fmt"
)

type Annotation struct {
	TicketID string
	FilePath string
	LineNum  int
	Context  string
}

func (s *Store) ensureAnnotationSchemaReady(ctx context.Context) error {
	s.annotationSchemaOnce.Do(func() {
		db, err := s.openDB()
		if err != nil {
			s.annotationSchemaErr = err
			return
		}
		defer db.Close()
		s.annotationSchemaErr = ensureAnnotationSchema(db)
	})
	return s.annotationSchemaErr
}

func (s *Store) UpsertAnnotations(ctx context.Context, annotations []Annotation) error {
	s.annotationMu.Lock()
	defer s.annotationMu.Unlock()

	if err := s.ensureAnnotationSchemaReady(ctx); err != nil {
		return err
	}

	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM annotations`); err != nil {
		return fmt.Errorf("clearing annotations: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO annotations (ticket_id, file_path, line_num, context) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range annotations {
		if _, err := stmt.ExecContext(ctx, a.TicketID, a.FilePath, a.LineNum, a.Context); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetAnnotationsByTicket(ctx context.Context, ticketID string) ([]Annotation, error) {
	s.annotationMu.Lock()
	defer s.annotationMu.Unlock()

	if err := s.ensureAnnotationSchemaReady(ctx); err != nil {
		return nil, err
	}

	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT ticket_id, file_path, line_num, context FROM annotations WHERE ticket_id = ? ORDER BY file_path ASC, line_num ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.TicketID, &a.FilePath, &a.LineNum, &a.Context); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAnnotationsByFile(ctx context.Context, filePath string) ([]Annotation, error) {
	s.annotationMu.Lock()
	defer s.annotationMu.Unlock()

	if err := s.ensureAnnotationSchemaReady(ctx); err != nil {
		return nil, err
	}

	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT ticket_id, file_path, line_num, context FROM annotations WHERE file_path = ? ORDER BY line_num ASC`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Annotation
	for rows.Next() {
		var a Annotation
		if err := rows.Scan(&a.TicketID, &a.FilePath, &a.LineNum, &a.Context); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func ensureAnnotationSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS annotations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ticket_id  TEXT NOT NULL,
			file_path  TEXT NOT NULL,
			line_num   INTEGER NOT NULL,
			context    TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_annotations_ticket ON annotations(ticket_id);
	`)
	return err
}
