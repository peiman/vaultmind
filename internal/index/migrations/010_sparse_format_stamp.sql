-- +goose Up
-- One stamp the embed path reads, separate from the goose schema version.
-- Missing means stored sparse vectors still count special tokens.
-- The value is written by the embed path after a successful sparse pass,
-- never inferred from which token ids a map happens to contain.
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE meta;
