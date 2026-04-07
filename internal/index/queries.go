package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

// QueryNotesByNormalized searches for notes whose title or alias, when hyphens and
// underscores are replaced with spaces and lowercased, matches the given normalized input.
func (d *DB) QueryNotesByNormalized(normalized string) ([]NoteRow, error) {
	q := `SELECT ` + noteColumns + ` FROM notes
		WHERE LOWER(REPLACE(REPLACE(title, '-', ' '), '_', ' ')) = ?
		UNION
		SELECT ` + noteColumns + ` FROM notes
		WHERE id IN (
			SELECT note_id FROM aliases
			WHERE LOWER(REPLACE(REPLACE(alias, '-', ' '), '_', ' ')) = ?
		)`
	rows, err := d.Query(q, normalized, normalized)
	if err != nil {
		return nil, fmt.Errorf("querying notes by normalized: %w", err)
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

// FullNote contains all data for a single note.
type FullNote struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Path        string                 `json:"path"`
	Title       string                 `json:"title"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	Body        string                 `json:"body,omitempty"`
	Headings    []HeadingRow           `json:"headings,omitempty"`
	Blocks      []BlockRow             `json:"blocks,omitempty"`
	IsDomain    bool                   `json:"is_domain_note"`
	Aliases     []string               `json:"-"`
	Tags        []string               `json:"-"`
}

// HeadingRow represents a heading in query results.
type HeadingRow struct {
	Level int    `json:"level"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// BlockRow represents a block ID in query results.
type BlockRow struct {
	BlockID string `json:"block_id"`
	Heading string `json:"heading,omitempty"`
	Line    int    `json:"line"`
}

// QueryFullNote returns complete note data including body, headings, blocks, aliases, tags.
// Uses GROUP_CONCAT subqueries to fold aliases and tags into the main note query,
// reducing the number of DB round-trips from 6 to 4.
func (d *DB) QueryFullNote(id string) (*FullNote, error) {
	var n FullNote
	var noteType, title, status, created, updated, body sql.NullString
	var aliasesCSV, tagsCSV sql.NullString
	err := d.QueryRow(`
		SELECT n.id, n.type, n.title, n.path, n.status, n.created, n.updated, n.body_text, n.is_domain,
			(SELECT GROUP_CONCAT(alias, char(31)) FROM aliases WHERE note_id = n.id) AS aliases_csv,
			(SELECT GROUP_CONCAT(tag, char(31)) FROM tags WHERE note_id = n.id) AS tags_csv
		FROM notes n
		WHERE n.id = ?`, id,
	).Scan(&n.ID, &noteType, &title, &n.Path, &status, &created, &updated, &body, &n.IsDomain,
		&aliasesCSV, &tagsCSV)
	if err == sql.ErrNoRows {
		return nil, nil //nolint:nilnil // not found
	}
	if err != nil {
		return nil, fmt.Errorf("querying full note: %w", err)
	}

	n.Type = noteType.String
	n.Title = title.String
	n.Body = body.String
	n.Frontmatter = make(map[string]interface{})
	if status.Valid && status.String != "" {
		n.Frontmatter["status"] = status.String
	}
	if created.Valid && created.String != "" {
		n.Frontmatter["created"] = created.String
	}
	if updated.Valid && updated.String != "" {
		n.Frontmatter["updated"] = updated.String
	}

	if aliasesCSV.Valid && aliasesCSV.String != "" {
		n.Aliases = strings.Split(aliasesCSV.String, "\x1F")
		n.Frontmatter["aliases"] = n.Aliases
	}
	if tagsCSV.Valid && tagsCSV.String != "" {
		n.Tags = strings.Split(tagsCSV.String, "\x1F")
		n.Frontmatter["tags"] = n.Tags
	}

	d.loadFrontmatterKV(&n)
	d.loadHeadings(&n)
	d.loadBlocks(&n)

	return &n, nil
}

func (d *DB) loadFrontmatterKV(n *FullNote) {
	rows, err := d.Query("SELECT key, value_json FROM frontmatter_kv WHERE note_id = ?", n.ID)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k, vJSON string
		if scanErr := rows.Scan(&k, &vJSON); scanErr == nil {
			var v interface{}
			if jsonErr := json.Unmarshal([]byte(vJSON), &v); jsonErr == nil {
				n.Frontmatter[k] = v
			}
		}
	}
}

func (d *DB) loadHeadings(n *FullNote) {
	rows, err := d.Query("SELECT level, title, heading_slug FROM headings WHERE note_id = ?", n.ID)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var h HeadingRow
		if scanErr := rows.Scan(&h.Level, &h.Title, &h.Slug); scanErr == nil {
			n.Headings = append(n.Headings, h)
		}
	}
}

func (d *DB) loadBlocks(n *FullNote) {
	rows, err := d.Query("SELECT block_id, heading, start_line FROM blocks WHERE note_id = ?", n.ID)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var b BlockRow
		var heading sql.NullString
		if scanErr := rows.Scan(&b.BlockID, &heading, &b.Line); scanErr == nil {
			b.Heading = heading.String
			n.Blocks = append(n.Blocks, b)
		}
	}
}
