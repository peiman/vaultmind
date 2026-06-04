-- +goose Up
ALTER TABLE notes ADD COLUMN last_accessed_at TEXT;
ALTER TABLE notes ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0;
