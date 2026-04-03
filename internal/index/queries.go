package index

import (
	"database/sql"
	"fmt"
)

// NoteRow is a lightweight note record returned by query methods.
type NoteRow struct {
	ID       string
	Type     string
	Title    string
	Path     string
	Status   string
	IsDomain bool
}

func scanNoteRow(row *sql.Row) (*NoteRow, error) {
	var n NoteRow
	var noteType, title, status sql.NullString
	err := row.Scan(&n.ID, &noteType, &title, &n.Path, &status, &n.IsDomain)
	if err == sql.ErrNoRows {
		return nil, nil //nolint:nilnil // nil,nil is the idiomatic "not found" for single-row queries
	}
	if err != nil {
		return nil, fmt.Errorf("scanning note row: %w", err)
	}
	n.Type = noteType.String
	n.Title = title.String
	n.Status = status.String
	return &n, nil
}

func scanNoteRows(rows *sql.Rows) ([]NoteRow, error) {
	defer func() { _ = rows.Close() }()
	var result []NoteRow
	for rows.Next() {
		var n NoteRow
		var noteType, title, status sql.NullString
		if err := rows.Scan(&n.ID, &noteType, &title, &n.Path, &status, &n.IsDomain); err != nil {
			return nil, fmt.Errorf("scanning note row: %w", err)
		}
		n.Type = noteType.String
		n.Title = title.String
		n.Status = status.String
		result = append(result, n)
	}
	return result, rows.Err()
}

const noteColumns = "id, type, title, path, status, is_domain"

// QueryNoteByID returns the note with the given ID, or nil if not found.
func (d *DB) QueryNoteByID(id string) (*NoteRow, error) {
	row := d.QueryRow("SELECT "+noteColumns+" FROM notes WHERE id = ?", id)
	return scanNoteRow(row)
}

// QueryNoteByPath returns the note at the given vault-relative path, or nil.
func (d *DB) QueryNoteByPath(path string) (*NoteRow, error) {
	row := d.QueryRow("SELECT "+noteColumns+" FROM notes WHERE path = ?", path)
	return scanNoteRow(row)
}

// QueryNotesByTitle returns notes matching the given title.
// If caseInsensitive is true, uses LOWER() comparison.
func (d *DB) QueryNotesByTitle(title string, caseInsensitive bool) ([]NoteRow, error) {
	var q string
	if caseInsensitive {
		q = "SELECT " + noteColumns + " FROM notes WHERE LOWER(title) = LOWER(?)"
	} else {
		q = "SELECT " + noteColumns + " FROM notes WHERE title = ?"
	}
	rows, err := d.Query(q, title)
	if err != nil {
		return nil, fmt.Errorf("querying notes by title: %w", err)
	}
	return scanNoteRows(rows)
}

// QueryNotesByAlias returns notes whose aliases match the given string.
// If normalized is true, compares against alias_normalized (lowercase, whitespace-collapsed).
func (d *DB) QueryNotesByAlias(alias string, normalized bool) ([]NoteRow, error) {
	var q string
	if normalized {
		q = `SELECT ` + noteColumns + ` FROM notes
			WHERE id IN (SELECT note_id FROM aliases WHERE alias_normalized = LOWER(?))`
	} else {
		q = `SELECT ` + noteColumns + ` FROM notes
			WHERE id IN (SELECT note_id FROM aliases WHERE alias = ?)`
	}
	rows, err := d.Query(q, alias)
	if err != nil {
		return nil, fmt.Errorf("querying notes by alias: %w", err)
	}
	return scanNoteRows(rows)
}
