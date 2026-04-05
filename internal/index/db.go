// Package index provides the SQLite-backed derived index for VaultMind.
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// DB wraps *sql.DB with schema initialization and VaultMind-specific helpers.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at dbPath, creates the parent
// directory if needed, applies the full VaultMind schema, and configures
// pragmas (WAL mode, foreign key enforcement).
func Open(dbPath string) (*DB, error) {
	cleanPath := filepath.Clean(dbPath)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0o750); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", cleanPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	// Single writer — one connection avoids locking issues.
	sqlDB.SetMaxOpenConns(1)

	d := &DB{db: sqlDB}
	if err := d.applySchema(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

// Exec executes a query that doesn't return rows.
func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

// QueryRow executes a query that returns at most one row.
func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.db.QueryRow(query, args...)
}

// Query executes a query that returns rows.
func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

// Begin starts a transaction.
func (d *DB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

// NoteHashInfo holds the content hash and modification time for a note.
type NoteHashInfo struct {
	Hash  string
	MTime int64
}

// NoteHashes returns a map of note path → NoteHashInfo for all notes in the
// database. Used by the incremental indexer to detect changed and deleted notes.
func (d *DB) NoteHashes() (map[string]NoteHashInfo, error) {
	rows, err := d.Query("SELECT path, hash, mtime FROM notes")
	if err != nil {
		return nil, fmt.Errorf("querying note hashes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]NoteHashInfo)
	for rows.Next() {
		var path, hash string
		var mtime int64
		if err := rows.Scan(&path, &hash, &mtime); err != nil {
			return nil, fmt.Errorf("scanning note hash: %w", err)
		}
		result[path] = NoteHashInfo{Hash: hash, MTime: mtime}
	}
	return result, rows.Err()
}

// UpdateMTime updates the mtime column for the note at the given path.
// Used when a file's content hash is unchanged but its mtime has changed.
func (d *DB) UpdateMTime(path string, mtime int64) error {
	_, err := d.Exec("UPDATE notes SET mtime = ? WHERE path = ?", mtime, path)
	return err
}

func (d *DB) applySchema() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := d.db.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS notes (
			rowid     INTEGER PRIMARY KEY AUTOINCREMENT,
			id        TEXT NOT NULL UNIQUE,
			path      TEXT NOT NULL UNIQUE,
			title     TEXT,
			type      TEXT,
			status    TEXT,
			created   TEXT,
			updated   TEXT,
			body_text TEXT,
			hash      TEXT NOT NULL,
			mtime     INTEGER NOT NULL,
			is_domain BOOLEAN NOT NULL DEFAULT FALSE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(type)`,

		`CREATE TABLE IF NOT EXISTS aliases (
			note_id          TEXT NOT NULL REFERENCES notes(id),
			alias            TEXT NOT NULL,
			alias_normalized TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_aliases_normalized ON aliases(alias_normalized)`,
		`CREATE INDEX IF NOT EXISTS idx_aliases_note       ON aliases(note_id)`,

		`CREATE TABLE IF NOT EXISTS tags (
			note_id TEXT NOT NULL REFERENCES notes(id),
			tag     TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag)`,

		`CREATE TABLE IF NOT EXISTS frontmatter_kv (
			note_id    TEXT NOT NULL REFERENCES notes(id),
			key        TEXT NOT NULL,
			value_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fmkv_note ON frontmatter_kv(note_id)`,

		`CREATE TABLE IF NOT EXISTS links (
			src_note_id TEXT NOT NULL,
			dst_note_id TEXT,
			dst_raw     TEXT NOT NULL,
			edge_type   TEXT NOT NULL,
			target_kind TEXT,
			heading     TEXT,
			block_id    TEXT,
			resolved    BOOLEAN NOT NULL DEFAULT FALSE,
			confidence  TEXT NOT NULL DEFAULT 'high',
			origin      TEXT,
			weight      REAL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_links_src          ON links(src_note_id)`,
		`CREATE INDEX IF NOT EXISTS idx_links_dst          ON links(dst_note_id)`,
		`CREATE INDEX IF NOT EXISTS idx_links_edge_type    ON links(edge_type)`,
		`CREATE INDEX IF NOT EXISTS idx_links_confidence   ON links(confidence)`,
		`CREATE INDEX IF NOT EXISTS idx_links_src_resolved ON links(src_note_id, resolved)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_links_unique ON links(src_note_id, dst_note_id, edge_type, dst_raw)`,

		`CREATE TABLE IF NOT EXISTS blocks (
			note_id    TEXT NOT NULL REFERENCES notes(id),
			block_id   TEXT NOT NULL,
			heading    TEXT,
			start_line INTEGER NOT NULL,
			end_line   INTEGER
		)`,

		`CREATE TABLE IF NOT EXISTS headings (
			note_id      TEXT NOT NULL REFERENCES notes(id),
			heading_slug TEXT NOT NULL,
			level        INTEGER NOT NULL,
			title        TEXT NOT NULL
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS fts_notes USING fts5(
			note_id UNINDEXED,
			title,
			body_text
		)`,

		`CREATE TABLE IF NOT EXISTS generated_sections (
			note_id     TEXT NOT NULL REFERENCES notes(id),
			section_key TEXT NOT NULL,
			checksum    TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			PRIMARY KEY (note_id, section_key)
		)`,
	}

	for _, stmt := range stmts {
		if _, err := d.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing DDL: %w\nstatement: %s", err, stmt)
		}
	}
	return nil
}
