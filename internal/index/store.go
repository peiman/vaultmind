package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// NoteRecord is the storage-ready representation of a parsed note.
// The indexer builds this from parser.ParsedNote + file metadata.
type NoteRecord struct {
	ID       string
	Path     string
	Title    string
	Type     string
	Status   string
	Created  string
	Updated  string
	BodyText string
	Hash     string
	MTime    int64
	IsDomain bool
	Aliases  []string
	Tags     []string
	ExtraKV  map[string]interface{}
	Links    []LinkRecord
	Headings []HeadingRecord
	Blocks   []BlockRecord
}

// LinkRecord represents a single outbound edge for storage.
type LinkRecord struct {
	DstNoteID  string
	DstRaw     string
	EdgeType   string
	TargetKind string
	Heading    string
	BlockID    string
	Resolved   bool
	Confidence string
	Origin     string
	Weight     float64
}

// HeadingRecord represents a heading for storage.
type HeadingRecord struct {
	Slug  string
	Level int
	Title string
}

// BlockRecord represents a block ID anchor for storage.
type BlockRecord struct {
	BlockID   string
	Heading   string
	StartLine int
	EndLine   int
}

// StoreNote deletes all existing rows for the note, then inserts fresh rows
// into every table within a single transaction (delete-before-reinsert).
// StoreNote stores a note within its own transaction.
func StoreNote(d *DB, rec NoteRecord) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := StoreNoteInTx(tx, rec); err != nil {
		return err
	}
	return tx.Commit()
}

// StoreNoteInTx stores a note within an existing transaction.
// Used by Rebuild for batch transactions.
func StoreNoteInTx(tx *sql.Tx, rec NoteRecord) error {
	noteID := rec.ID

	// Delete dependent rows first
	for _, table := range []string{"aliases", "tags", "frontmatter_kv", "blocks", "headings", "generated_sections"} {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE note_id = ?", table), noteID); err != nil {
			return fmt.Errorf("deleting from %s: %w", table, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM links WHERE src_note_id = ?", noteID); err != nil {
		return fmt.Errorf("deleting from links: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM fts_notes WHERE note_id = ?", noteID); err != nil {
		return fmt.Errorf("deleting from fts_notes: %w", err)
	}

	// Upsert notes row
	if err := upsertNote(tx, rec); err != nil {
		return err
	}

	// Aliases
	for _, alias := range rec.Aliases {
		trimmed := strings.TrimSpace(alias)
		normalized := normalizeAlias(trimmed)
		if _, err := tx.Exec(
			"INSERT INTO aliases (note_id, alias, alias_normalized) VALUES (?, ?, ?)",
			noteID, trimmed, normalized,
		); err != nil {
			return fmt.Errorf("inserting alias %q: %w", alias, err)
		}
	}

	// Tags
	for _, tag := range rec.Tags {
		if _, err := tx.Exec("INSERT INTO tags (note_id, tag) VALUES (?, ?)", noteID, tag); err != nil {
			return fmt.Errorf("inserting tag %q: %w", tag, err)
		}
	}

	// Frontmatter extra key-value pairs
	for k, v := range rec.ExtraKV {
		encoded, encErr := json.Marshal(v)
		if encErr != nil {
			return fmt.Errorf("encoding frontmatter key %q: %w", k, encErr)
		}
		if _, err := tx.Exec(
			"INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES (?, ?, ?)",
			noteID, k, string(encoded),
		); err != nil {
			return fmt.Errorf("inserting frontmatter_kv %q: %w", k, err)
		}
	}

	// Links
	for _, link := range rec.Links {
		var dstNoteID interface{}
		if link.DstNoteID != "" {
			dstNoteID = link.DstNoteID
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO links
			  (src_note_id, dst_note_id, dst_raw, edge_type, target_kind, heading,
			   block_id, resolved, confidence, origin, weight)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			noteID, dstNoteID, link.DstRaw, link.EdgeType,
			nullable(link.TargetKind), nullable(link.Heading),
			nullable(link.BlockID), link.Resolved, link.Confidence,
			nullable(link.Origin), link.Weight,
		); err != nil {
			return fmt.Errorf("inserting link %q: %w", link.DstRaw, err)
		}
	}

	// Headings
	for _, h := range rec.Headings {
		if _, err := tx.Exec(
			"INSERT INTO headings (note_id, heading_slug, level, title) VALUES (?, ?, ?, ?)",
			noteID, h.Slug, h.Level, h.Title,
		); err != nil {
			return fmt.Errorf("inserting heading %q: %w", h.Slug, err)
		}
	}

	// Blocks
	for _, b := range rec.Blocks {
		var endLine interface{}
		if b.EndLine > 0 {
			endLine = b.EndLine
		}
		if _, err := tx.Exec(
			"INSERT INTO blocks (note_id, block_id, heading, start_line, end_line) VALUES (?, ?, ?, ?, ?)",
			noteID, b.BlockID, nullable(b.Heading), b.StartLine, endLine,
		); err != nil {
			return fmt.Errorf("inserting block %q: %w", b.BlockID, err)
		}
	}

	// FTS
	if _, err := tx.Exec(
		"INSERT INTO fts_notes (note_id, title, body_text) VALUES (?, ?, ?)",
		noteID, rec.Title, rec.BodyText,
	); err != nil {
		return fmt.Errorf("inserting fts_notes: %w", err)
	}

	return nil
}

func upsertNote(tx *sql.Tx, rec NoteRecord) error {
	_, err := tx.Exec(`
		INSERT INTO notes (id, path, title, type, status, created, updated,
		  body_text, hash, mtime, is_domain)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  path=excluded.path, title=excluded.title, type=excluded.type,
		  status=excluded.status, created=excluded.created, updated=excluded.updated,
		  body_text=excluded.body_text, hash=excluded.hash, mtime=excluded.mtime,
		  is_domain=excluded.is_domain`,
		rec.ID, rec.Path, nullable(rec.Title), nullable(rec.Type),
		nullable(rec.Status), nullable(rec.Created), nullable(rec.Updated),
		nullable(rec.BodyText), rec.Hash, rec.MTime, rec.IsDomain,
	)
	if err != nil {
		return fmt.Errorf("upserting note %q: %w", rec.ID, err)
	}
	return nil
}

// DeleteNoteByPath removes a note and all its dependent rows from every table
// within a single transaction. It is used by the incremental indexer to clean
// up notes whose source files no longer exist on disk.
func DeleteNoteByPath(d *DB, path string) error {
	var noteID string
	err := d.QueryRow("SELECT id FROM notes WHERE path = ?", path).Scan(&noteID)
	if err != nil {
		return fmt.Errorf("finding note by path %q: %w", path, err)
	}
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("beginning delete transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	dependentDeletes := []struct {
		stmt  string
		label string
	}{
		{"DELETE FROM aliases WHERE note_id = ?", "aliases"},
		{"DELETE FROM tags WHERE note_id = ?", "tags"},
		{"DELETE FROM frontmatter_kv WHERE note_id = ?", "frontmatter_kv"},
		{"DELETE FROM blocks WHERE note_id = ?", "blocks"},
		{"DELETE FROM headings WHERE note_id = ?", "headings"},
		{"DELETE FROM generated_sections WHERE note_id = ?", "generated_sections"},
		{"DELETE FROM fts_notes WHERE note_id = ?", "fts_notes"},
	}
	for _, dep := range dependentDeletes {
		if _, err := tx.Exec(dep.stmt, noteID); err != nil {
			return fmt.Errorf("deleting from %s: %w", dep.label, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM links WHERE src_note_id = ?", noteID); err != nil {
		return fmt.Errorf("deleting outbound links: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM links WHERE dst_note_id = ?", noteID); err != nil {
		return fmt.Errorf("deleting inbound links: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM notes WHERE id = ?", noteID); err != nil {
		return fmt.Errorf("deleting note: %w", err)
	}
	return tx.Commit()
}

func normalizeAlias(alias string) string {
	return strings.Join(strings.Fields(strings.ToLower(alias)), " ")
}

func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
