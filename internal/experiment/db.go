// Package experiment provides the SQLite-backed database for VaultMind experiment data.
package experiment

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// migrations is an ordered list of SQL statements to apply for each schema version.
// Index 0 = migration from version 0 → 1.
var migrations = []string{
	// Version 1: initial schema — sessions, events, outcomes.
	`CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    vault_path TEXT NOT NULL,
    started_at TEXT NOT NULL,
    ended_at   TEXT
);

CREATE TABLE IF NOT EXISTS events (
    event_id        TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(session_id),
    event_type      TEXT NOT NULL,
    timestamp       TEXT NOT NULL,
    vault_path      TEXT NOT NULL,
    query_text      TEXT,
    query_mode      TEXT,
    primary_variant TEXT,
    event_data      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_session   ON events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_type      ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

CREATE TABLE IF NOT EXISTS outcomes (
    outcome_id  TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL REFERENCES events(event_id),
    note_id     TEXT NOT NULL,
    variant     TEXT NOT NULL,
    rank        INTEGER NOT NULL,
    accessed_at TEXT NOT NULL,
    session_id  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_outcomes_event ON outcomes(event_id);
CREATE INDEX IF NOT EXISTS idx_outcomes_note  ON outcomes(note_id);`,
	// Version 2: per_note_stats view. Unnests event_data.variants.*.results[]
	// across search/ask/context_pack events and aggregates per-note metrics.
	// DISTINCT on (event_id, note_id) so a note appearing in multiple variants
	// of the same event counts once.
	`CREATE VIEW IF NOT EXISTS per_note_stats AS
WITH hits AS (
    SELECT DISTINCT
        e.event_id,
        e.timestamp,
        json_extract(h.value, '$.note_id') AS note_id
    FROM events e,
         json_each(e.event_data, '$.variants') v,
         json_each(v.value, '$.results') h
    WHERE e.event_type IN ('search', 'ask', 'context_pack')
      AND json_valid(e.event_data)
      AND json_extract(h.value, '$.note_id') IS NOT NULL
)
SELECT
    note_id,
    COUNT(*) AS retrieval_count_total,
    MIN(timestamp) AS first_retrieved_ts,
    MAX(timestamp) AS last_retrieved_ts
FROM hits
GROUP BY note_id;`,
	// Version 3: session_gaps view. For each session, reports the previous
	// session and the inter-session interval in seconds. Feeds compressed-
	// idle-time analysis (gamma parameter). First session's prev_* fields
	// and gap_seconds are NULL.
	`CREATE VIEW IF NOT EXISTS session_gaps AS
SELECT
    session_id,
    started_at,
    LAG(session_id) OVER (ORDER BY started_at) AS prev_session_id,
    LAG(started_at) OVER (ORDER BY started_at) AS prev_started_at,
    CAST(
        ROUND((julianday(started_at) - julianday(LAG(started_at) OVER (ORDER BY started_at))) * 86400)
        AS INTEGER
    ) AS gap_seconds
FROM sessions;`,
	// Version 4: caller attribution on sessions. caller is a short label
	// identifying the invoking agent (e.g. "workhorse-persona-hook",
	// "claude-code", "cli"); caller_meta is a JSON blob for flexible
	// attribution (project_dir, pid, env-var snapshot). Existing rows stay
	// NULL — reporters treat NULL as "unknown" rather than failing.
	`ALTER TABLE sessions ADD COLUMN caller TEXT;
ALTER TABLE sessions ADD COLUMN caller_meta TEXT;`,
}

// DB wraps *sql.DB with schema initialization and experiment-specific helpers.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database at dbPath, creates the parent
// directory if needed, applies pragmas (WAL mode, foreign key enforcement),
// and runs pending migrations using PRAGMA user_version for versioning.
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
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

// Exec executes a query that doesn't return rows.
func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

// QueryRow executes a query that returns at most one row.
func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

// Query executes a query that returns rows.
func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

// Begin starts a transaction.
func (d *DB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

// schemaVersion reads the current schema version from PRAGMA user_version.
func (d *DB) schemaVersion() (int, error) {
	var version int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("reading user_version: %w", err)
	}
	return version, nil
}

// setSchemaVersion writes the schema version to PRAGMA user_version.
func (d *DB) setSchemaVersion(v int) error {
	// PRAGMA user_version does not support parameter binding; the value must be
	// embedded directly. v is always an internal integer so this is safe.
	if _, err := d.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v)); err != nil {
		return fmt.Errorf("setting user_version to %d: %w", v, err)
	}
	return nil
}

// migrate applies any pending migrations in order, advancing the schema version
// after each one succeeds.
func (d *DB) migrate() error {
	current, err := d.schemaVersion()
	if err != nil {
		return err
	}

	for i := current; i < len(migrations); i++ {
		if _, err := d.db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("applying migration %d: %w", i+1, err)
		}
		if err := d.setSchemaVersion(i + 1); err != nil {
			return err
		}
	}
	return nil
}

// applyPragmas sets WAL mode and enables foreign key enforcement.
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
