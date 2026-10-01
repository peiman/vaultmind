-- +goose Up
-- Migration 009: the sections of long notes.
--
-- BGE-M3 embeds at most 8,192 tokens. An imported doc page runs 25–40k, so
-- everything past the window was invisible to semantic search, and one vector
-- over twenty topics matched each of them weakly. A note longer than the
-- window is now cut at its headings (internal/section); each part is a row
-- here, keyed <note_id>#<anchor>, and will be a retrieval unit of its own.
--
-- Only long notes have rows. A note that fits the window keeps its single
-- row in notes and its embeddings there, untouched — upgrading re-embeds only
-- the long notes, not the vault.
--
-- No ColBERT column: on long documents ColBERT adds 0.2 nDCG@10 over dense +
-- sparse (BGE-M3, MLDR) at ~4 KB per token, so sections get dense and sparse
-- only (decided 2026-10-01).
--
-- note_hash is the hash of the note text a section was cut from. A binary
-- from before this migration re-indexes an edited note without touching its
-- sections, and the newer indexer then skips that note as unchanged; a reader
-- tells the stale sections apart by note_hash != notes.hash.
CREATE TABLE IF NOT EXISTS sections (
    id               TEXT PRIMARY KEY,
    note_id          TEXT NOT NULL REFERENCES notes(id),
    note_hash        TEXT NOT NULL,
    ordinal          INTEGER NOT NULL,
    anchor           TEXT NOT NULL,
    heading_path     TEXT NOT NULL,
    body             TEXT NOT NULL,
    embedding        BLOB,
    sparse_embedding BLOB
);

CREATE INDEX IF NOT EXISTS idx_sections_note_id ON sections(note_id);

-- +goose Down
DROP INDEX IF EXISTS idx_sections_note_id;
DROP TABLE IF EXISTS sections;
