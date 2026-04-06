-- +goose Up
CREATE TABLE IF NOT EXISTS notes (
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
);
CREATE INDEX IF NOT EXISTS idx_notes_type ON notes(type);

CREATE TABLE IF NOT EXISTS aliases (
	note_id          TEXT NOT NULL REFERENCES notes(id),
	alias            TEXT NOT NULL,
	alias_normalized TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_aliases_normalized ON aliases(alias_normalized);
CREATE INDEX IF NOT EXISTS idx_aliases_note       ON aliases(note_id);

CREATE TABLE IF NOT EXISTS tags (
	note_id TEXT NOT NULL REFERENCES notes(id),
	tag     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);

CREATE TABLE IF NOT EXISTS frontmatter_kv (
	note_id    TEXT NOT NULL REFERENCES notes(id),
	key        TEXT NOT NULL,
	value_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_fmkv_note ON frontmatter_kv(note_id);

CREATE TABLE IF NOT EXISTS links (
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
);
CREATE INDEX IF NOT EXISTS idx_links_src          ON links(src_note_id);
CREATE INDEX IF NOT EXISTS idx_links_dst          ON links(dst_note_id);
CREATE INDEX IF NOT EXISTS idx_links_edge_type    ON links(edge_type);
CREATE INDEX IF NOT EXISTS idx_links_confidence   ON links(confidence);
CREATE INDEX IF NOT EXISTS idx_links_src_resolved ON links(src_note_id, resolved);
CREATE UNIQUE INDEX IF NOT EXISTS idx_links_unique ON links(src_note_id, dst_note_id, edge_type, dst_raw);

CREATE TABLE IF NOT EXISTS blocks (
	note_id    TEXT NOT NULL REFERENCES notes(id),
	block_id   TEXT NOT NULL,
	heading    TEXT,
	start_line INTEGER NOT NULL,
	end_line   INTEGER
);

CREATE TABLE IF NOT EXISTS headings (
	note_id      TEXT NOT NULL REFERENCES notes(id),
	heading_slug TEXT NOT NULL,
	level        INTEGER NOT NULL,
	title        TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_notes USING fts5(
	note_id UNINDEXED,
	title,
	body_text
);

CREATE TABLE IF NOT EXISTS generated_sections (
	note_id     TEXT NOT NULL REFERENCES notes(id),
	section_key TEXT NOT NULL,
	checksum    TEXT NOT NULL,
	updated_at  TEXT NOT NULL,
	PRIMARY KEY (note_id, section_key)
);
