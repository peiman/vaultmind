# Storage Model

> See also: [data model](03-data-model.md), [memory model](05-memory-model.md), [validation rules](13-validation-rules.md)

## Overview

SQLite is the local derived index. It can always be rebuilt from the vault via `vaultmind index`.

## Schema

```sql
-- Core note index
-- Uses integer rowid (implicit) for FTS compatibility.
-- Domain notes: id = frontmatter id. Unstructured notes: id = '_path:<vault-relative-path>'.
CREATE TABLE notes (
  rowid         INTEGER PRIMARY KEY AUTOINCREMENT,
  id            TEXT NOT NULL UNIQUE,
  path          TEXT NOT NULL UNIQUE,
  title         TEXT,
  type          TEXT,
  status        TEXT,
  created       TEXT,
  updated       TEXT,
  body_text     TEXT,                  -- plain text body (markdown stripped) for FTS
  hash          TEXT NOT NULL,         -- SHA-256 of file content
  mtime         INTEGER NOT NULL,     -- Unix timestamp
  is_domain     BOOLEAN NOT NULL DEFAULT FALSE
);

-- Aliases for entity resolution
CREATE TABLE aliases (
  note_id       TEXT NOT NULL REFERENCES notes(id),
  alias         TEXT NOT NULL,
  alias_normalized TEXT NOT NULL       -- lowercase, whitespace-collapsed
);
CREATE INDEX idx_aliases_normalized ON aliases(alias_normalized);

-- Tags
CREATE TABLE tags (
  note_id       TEXT NOT NULL REFERENCES notes(id),
  tag           TEXT NOT NULL
);
CREATE INDEX idx_tags_tag ON tags(tag);

-- Arbitrary frontmatter key-value pairs (domain-tier fields not in dedicated columns)
CREATE TABLE frontmatter_kv (
  note_id       TEXT NOT NULL REFERENCES notes(id),
  key           TEXT NOT NULL,
  value_json    TEXT NOT NULL          -- JSON-encoded value
);
CREATE INDEX idx_fmkv_note ON frontmatter_kv(note_id);

-- Graph edges
CREATE TABLE links (
  src_note_id   TEXT NOT NULL,
  dst_note_id   TEXT,                  -- NULL if unresolved
  dst_raw       TEXT NOT NULL,         -- original reference text
  edge_type     TEXT NOT NULL,         -- explicit_link, explicit_embed, explicit_relation,
                                       -- alias_mention, tag_overlap, dataview_reference
  target_kind   TEXT,                  -- note, heading, block
  heading       TEXT,
  block_id      TEXT,
  resolved      BOOLEAN NOT NULL DEFAULT FALSE,
  confidence    TEXT NOT NULL DEFAULT 'high',
  origin        TEXT,                  -- e.g., "body:line:24", "frontmatter.related_ids"
  weight        REAL                   -- used for tag_overlap score
);
CREATE INDEX idx_links_src ON links(src_note_id);
CREATE INDEX idx_links_dst ON links(dst_note_id);
CREATE INDEX idx_links_edge_type ON links(edge_type);
CREATE INDEX idx_links_confidence ON links(confidence);
CREATE INDEX idx_links_src_resolved ON links(src_note_id, resolved);
CREATE UNIQUE INDEX idx_links_unique ON links(src_note_id, dst_note_id, edge_type, dst_raw);

CREATE INDEX idx_notes_type ON notes(type);
CREATE INDEX idx_aliases_note ON aliases(note_id);

-- Block references
CREATE TABLE blocks (
  note_id       TEXT NOT NULL REFERENCES notes(id),
  block_id      TEXT NOT NULL,
  heading       TEXT,
  start_line    INTEGER NOT NULL,
  end_line      INTEGER               -- NULL for single-line blocks (e.g., ^block-id on a paragraph)
);

-- Headings
CREATE TABLE headings (
  note_id       TEXT NOT NULL REFERENCES notes(id),
  heading_slug  TEXT NOT NULL,
  level         INTEGER NOT NULL,
  title         TEXT NOT NULL
);

-- Full-text search (standalone, not content-synced)
CREATE VIRTUAL TABLE fts_notes USING fts5 (
  note_id UNINDEXED,
  title,
  body_text
);

-- Generated section checksums
CREATE TABLE generated_sections (
  note_id       TEXT NOT NULL REFERENCES notes(id),
  section_key   TEXT NOT NULL,
  checksum      TEXT NOT NULL,         -- SHA-256 of content between markers
  updated_at    TEXT NOT NULL,
  PRIMARY KEY (note_id, section_key)
);
```

### `frontmatter_kv` Usage

This table stores domain-tier fields that don't have dedicated columns. For example, `owner_id`, `project_id`, `due`, `url`, `score`. Core and graph-tier fields use dedicated columns/tables (`notes.status`, `aliases`, `tags`). This enables queries like "find all notes where `owner_id` = X" without needing to know the schema at table-creation time.

## Indexing Strategy

### Full Rebuild

`vaultmind index` scans all files, parses, and rebuilds the entire database. Idempotent.

### Incremental Indexing

On subsequent runs, two-tier change detection:

1. **Fast path (mtime):** If file mtime unchanged since last index, skip.
2. **Hash verification:** If mtime changed, compute SHA-256 and compare to stored hash. If hash matches, update mtime only. If hash differs, re-parse and re-index.

Incremental indexing also detects deleted files (present in index but absent from filesystem) and removes their records.

**Re-index contract:** When a file is re-indexed, all its outbound edges in `links`, entries in `aliases`, `tags`, `frontmatter_kv`, `headings`, `blocks`, and `generated_sections` are deleted before reinsertion. This prevents duplicate accumulation. The unique index on `links` provides a safety net but the delete-before-reinsert pattern is the primary guarantee.

### FTS Maintenance

The `fts_notes` table is a standalone FTS5 table (not content-synced). During indexing, rows are inserted/deleted/updated in lockstep with the `notes` table. The `body_text` column stores markdown with formatting stripped (no frontmatter, no code fences, no HTML).
