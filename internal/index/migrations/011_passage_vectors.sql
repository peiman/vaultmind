-- +goose Up
CREATE TABLE IF NOT EXISTS passage_vectors (
    note_id TEXT NOT NULL REFERENCES notes(id),
    unit_idx INTEGER NOT NULL,
    start INTEGER NOT NULL,
    end INTEGER NOT NULL,
    dense BLOB NOT NULL,
    PRIMARY KEY (note_id, unit_idx)
);

-- Passage vectors belong to the current note embedding, including a full purge.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS passage_vectors_dense_update AFTER UPDATE OF embedding ON notes
WHEN OLD.embedding IS NOT NEW.embedding
BEGIN
    DELETE FROM passage_vectors WHERE note_id=NEW.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER passage_vectors_dense_update;
DROP TABLE passage_vectors;
