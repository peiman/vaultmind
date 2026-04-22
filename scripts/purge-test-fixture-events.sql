-- Purge test-fixture note_access events from the production experiment DB.
-- One-time cleanup for issue #19. Context: docs/cleanup/2026-04-22-purge-test-fixture-events.md.
--
-- Before running, back up the DB:
--   cp "$HOME/Library/Application Support/vaultmind/experiments.db" \
--      "$HOME/Library/Application Support/vaultmind/experiments.db.bak.$(date -u +%Y%m%dT%H%M%SZ)"
--
-- Apply:
--   sqlite3 "$HOME/Library/Application Support/vaultmind/experiments.db" \
--     < scripts/purge-test-fixture-events.sql
--
-- Idempotent: re-running removes zero rows.

SELECT 'before: note_access rows = ' || COUNT(*)
FROM events WHERE event_type = 'note_access';

-- The three fixture note IDs are defined only in test files under
-- internal/{memory,mutation,baseline,index,experiment,dev}/*_test.go.
-- No real vault contains any of them. Before #17 landed, tests wrote to
-- the production user-data directory; these rows are the leak.
DELETE FROM events
WHERE event_type = 'note_access'
  AND json_extract(event_data, '$.note_id') IN (
    'concept-alpha',
    'c-spreading',
    'does-not-exist'
  );

SELECT 'after:  note_access rows = ' || COUNT(*)
FROM events WHERE event_type = 'note_access';

SELECT 'after:  distinct note_ids = ' || COUNT(DISTINCT json_extract(event_data, '$.note_id'))
FROM events WHERE event_type = 'note_access';
