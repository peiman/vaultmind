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

// pathIDDependentTables are the tables whose note_id column references
// notes(id) and that StoreNoteInTx deletes-then-reinserts on every store.
// migrateNoteID reuses this list to evict the OLD id's dependent rows during
// an id migration so no orphans linger after the row is re-keyed. SSOT for
// "what hangs off a note id" — keep aligned with StoreNoteInTx's delete loop.
var pathIDDependentTables = []string{
	"aliases", "tags", "frontmatter_kv", "blocks", "headings", "generated_sections",
}

// StoreNoteInTx stores a note within an existing transaction.
// Used by Rebuild for batch transactions.
func StoreNoteInTx(tx *sql.Tx, rec NoteRecord) error {
	noteID := rec.ID

	// A note first indexed WITHOUT a frontmatter id is stored under a
	// path-derived `_path:<relpath>` id. When the file later GAINS a real id,
	// this store carries a NEW id for the SAME path. A plain upsert would then
	// INSERT a fresh row and hit `UNIQUE constraint failed: notes.path`, the
	// note would be skipped, and search would keep serving the stale `_path:`
	// row while content never updates (silent data loss). Treat this as an id
	// MIGRATION: re-key the existing row and carry access history forward
	// BEFORE the rest of the store proceeds under the new id.
	if err := migrateNoteID(tx, rec.Path, noteID); err != nil {
		return err
	}

	// Delete dependent rows first
	for _, table := range pathIDDependentTables {
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

	// Links — deduplicate by (dst_raw, edge_type) to prevent NULL dst_note_id
	// duplicates that SQLite's unique index treats as distinct (NULL != NULL).
	// Without dedup, ResolveLinks can only resolve one copy, leaving phantom
	// unresolved links.
	seenLinks := make(map[string]bool)
	for _, link := range rec.Links {
		linkKey := link.DstRaw + "\x00" + link.EdgeType
		if seenLinks[linkKey] {
			continue
		}
		seenLinks[linkKey] = true

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
		  is_domain=excluded.is_domain,
		  -- Clear stale embeddings when the note's content changes. Without
		  -- this, semantic retrieval keeps returning hits computed from the
		  -- old body_text. Embeddings are regenerated by the next
		  -- 'vaultmind index --embed' run (which only re-embeds rows whose
		  -- corresponding column is NULL).
		  embedding        = CASE WHEN notes.hash != excluded.hash THEN NULL ELSE notes.embedding END,
		  sparse_embedding = CASE WHEN notes.hash != excluded.hash THEN NULL ELSE notes.sparse_embedding END,
		  colbert_embedding= CASE WHEN notes.hash != excluded.hash THEN NULL ELSE notes.colbert_embedding END`,
		rec.ID, rec.Path, nullable(rec.Title), nullable(rec.Type),
		nullable(rec.Status), nullable(rec.Created), nullable(rec.Updated),
		nullable(rec.BodyText), rec.Hash, rec.MTime, rec.IsDomain,
	)
	if err != nil {
		return fmt.Errorf("upserting note %q: %w", rec.ID, err)
	}
	return nil
}

// migrateNoteID re-keys an existing note row when the file at `path` now
// declares a different id than the stored row (the `_path:` → real-id adoption
// case). It is a no-op when there is no stored row for the path, or when the
// stored id already equals newID.
//
// The migration is transactional and carries note_accesses (activation/recall
// history) forward to the new id; the scalar access columns survive implicitly
// because the notes row is updated in place rather than replaced. The OLD id's
// dependent rows are evicted here so the row's id can be re-keyed without
// orphaning them — StoreNoteInTx re-inserts them fresh under the new id.
//
// FK enforcement is deferred to commit (defer_foreign_keys) so the parent
// notes row and the child note_accesses rows can be re-keyed in the same
// transaction regardless of statement order; both reference the new id by the
// time the transaction commits.
func migrateNoteID(tx *sql.Tx, path, newID string) error {
	oldID, err := storedIDForPath(tx, path)
	if err != nil || oldID == "" || oldID == newID {
		return err
	}

	if _, err := tx.Exec("PRAGMA defer_foreign_keys=ON"); err != nil {
		return fmt.Errorf("deferring foreign keys for id migration: %w", err)
	}
	if err := evictOldIDDependents(tx, oldID); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE note_accesses SET note_id = ? WHERE note_id = ?", newID, oldID); err != nil {
		return fmt.Errorf("carrying note_accesses forward to %q: %w", newID, err)
	}
	if _, err := tx.Exec("UPDATE notes SET id = ? WHERE id = ?", newID, oldID); err != nil {
		return fmt.Errorf("re-keying note id %q -> %q: %w", oldID, newID, err)
	}
	return nil
}

// storedIDForPath returns the id currently stored for path, or "" when no row
// exists for it. A missing row is not an error — it is the common new-file case.
func storedIDForPath(tx *sql.Tx, path string) (string, error) {
	var id string
	err := tx.QueryRow("SELECT id FROM notes WHERE path = ?", path).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("looking up stored id for path %q: %w", path, err)
	}
	return id, nil
}

// evictOldIDDependents removes every dependent row keyed to oldID so the note
// row can be re-keyed without orphaning them. Outbound links and the FTS row
// key on columns outside pathIDDependentTables, so they are deleted explicitly.
// Inbound links (dst_note_id = oldID) belong to OTHER notes and are left for
// ResolveLinks to re-point at the new id.
func evictOldIDDependents(tx *sql.Tx, oldID string) error {
	for _, table := range pathIDDependentTables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE note_id = ?", table), oldID); err != nil {
			return fmt.Errorf("evicting old id from %s: %w", table, err)
		}
	}
	if _, err := tx.Exec("DELETE FROM links WHERE src_note_id = ?", oldID); err != nil {
		return fmt.Errorf("evicting old id outbound links: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM fts_notes WHERE note_id = ?", oldID); err != nil {
		return fmt.Errorf("evicting old id fts row: %w", err)
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
