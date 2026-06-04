-- +goose Up
ALTER TABLE notes ADD COLUMN sparse_embedding BLOB;
ALTER TABLE notes ADD COLUMN colbert_embedding BLOB;

-- +goose Down
-- SQLite does not support DROP COLUMN before 3.35.0
