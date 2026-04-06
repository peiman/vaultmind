// Package index provides the SQLite-backed derived index for VaultMind.
package index

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

//go:embed migrations/*.sql
var migrations embed.FS

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
	if err := d.applyPragmas(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("applying pragmas: %w", err)
	}
	if err := d.applyMigrations(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("applying migrations: %w", err)
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

func (d *DB) applyPragmas() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := d.db.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	return nil
}

func (d *DB) applyMigrations() error {
	goose.SetLogger(goose.NopLogger())
	migrationsFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("sub-filesystem for migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, d.db, migrationsFS)
	if err != nil {
		return fmt.Errorf("creating goose provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}
