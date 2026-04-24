-- +goose Up
--
-- Close the silent-failure class exposed by the 2026-04-24 ranking bug.
-- BGE-M3 writes all three modalities (dense + sparse + colbert) in lockstep;
-- any write that leaves a note with BGE-M3 dense (1024 float32 = 4096 bytes)
-- without both companions silently compresses hybrid RRF ranking for that
-- note. The retriever-side mean-of-present fix degrades gracefully but the
-- class of bug can still re-enter from any code path that bypasses the
-- indexer's embed loop. Enforce the invariant at the schema level.

-- +goose StatementBegin
-- Cleanup first: any existing row that violates the invariant gets its dense
-- NULLed so the next `vaultmind index --embed --model bge-m3` re-fills all
-- three atomically. Dropping the dense is cheap (re-embed is idempotent) and
-- strictly better than carrying a silently-degraded note forward.
UPDATE notes
SET embedding = NULL
WHERE embedding IS NOT NULL
  AND LENGTH(embedding) = 4096
  AND (sparse_embedding IS NULL OR colbert_embedding IS NULL);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bgem3_modality_parity_insert
BEFORE INSERT ON notes
FOR EACH ROW
WHEN NEW.embedding IS NOT NULL
  AND LENGTH(NEW.embedding) = 4096
  AND (NEW.sparse_embedding IS NULL OR NEW.colbert_embedding IS NULL)
BEGIN
  SELECT RAISE(ABORT, 'BGE-M3 modality parity: dense present requires sparse and colbert');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER bgem3_modality_parity_update
BEFORE UPDATE ON notes
FOR EACH ROW
WHEN NEW.embedding IS NOT NULL
  AND LENGTH(NEW.embedding) = 4096
  AND (NEW.sparse_embedding IS NULL OR NEW.colbert_embedding IS NULL)
BEGIN
  SELECT RAISE(ABORT, 'BGE-M3 modality parity: dense present requires sparse and colbert');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS bgem3_modality_parity_insert;
DROP TRIGGER IF EXISTS bgem3_modality_parity_update;
