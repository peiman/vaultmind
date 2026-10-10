-- +goose Up
-- Delivery uses the parsed body as written; body_text remains the search and
-- embedding input. NULL marks existing rows needing a source-file refresh.
-- A plain incremental index backfills these rows without touching embeddings.
ALTER TABLE notes ADD COLUMN body_raw TEXT;

-- +goose Down
ALTER TABLE notes DROP COLUMN body_raw;
