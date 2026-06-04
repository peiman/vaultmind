-- +goose Up
CREATE INDEX IF NOT EXISTS idx_notes_title ON notes(title COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS idx_aliases_alias ON aliases(alias COLLATE NOCASE);
