-- +goose Up
-- Migration 007: per-event access log with caller provenance.
--
-- Background: migration 004 added scalar (access_count, last_accessed_at)
-- to the notes table. RecordNoteAccess increments these on every access,
-- regardless of who fired the access. The 2026-05-01 inter-agent review
-- caught the blind spot: the SessionStart hook fires `vaultmind ask` and
-- `vaultmind self` queries that fan-out RecordNoteAccess across many
-- notes BEFORE the agent does any deliberate work. Result: `vaultmind
-- self` shows the harness's footprint, not the agent's engagement
-- footprint, undermining the proprioceptive value the command exists for.
--
-- Right-layer fix (per close-at-the-right-layer arc): access events
-- become first-class events with a caller dimension. The scalar columns
-- on notes stay for backward compatibility and fast lookup, but the new
-- events table is the source of truth for "who touched this note when".
-- Self can filter by caller; future ACT-R retrieval scoring (slice 5b')
-- can use the per-event timestamp history that ComputeRetrieval (in
-- internal/experiment/activation.go) already wants but couldn't have.
CREATE TABLE IF NOT EXISTS note_accesses (
    rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
    note_id     TEXT NOT NULL,
    caller      TEXT NOT NULL,
    accessed_at TEXT NOT NULL,
    FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_note_accesses_note_id ON note_accesses(note_id);
CREATE INDEX IF NOT EXISTS idx_note_accesses_accessed_at ON note_accesses(accessed_at);
CREATE INDEX IF NOT EXISTS idx_note_accesses_caller ON note_accesses(caller);
