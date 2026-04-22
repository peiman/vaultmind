# Purge: test-fixture `note_access` events from production experiment DB

**Date:** 2026-04-22 (UTC)
**Issue:** [#19](https://github.com/peiman/vaultmind/issues/19)
**Context:** [close comment on #16](https://github.com/peiman/vaultmind/issues/16#issuecomment-4295814438), [#17](https://github.com/peiman/vaultmind/issues/17) (root-cause fix)

## What was removed

259 `note_access` events where `note_id` ∈ `{concept-alpha, c-spreading, does-not-exist}`.

All three IDs are test fixtures defined only in `*_test.go` files — no real vault contains any of them. The events accumulated between 2026-04-21 and 2026-04-22 because tests were writing to the production user-data directory (`~/Library/Application Support/vaultmind/experiments.db`) instead of an isolated tmp directory.

Root cause: `internal/xdg/xdg.go:dataBase()` only respected `XDG_DATA_HOME` on Linux. On macOS it unconditionally returned `$HOME/Library/Application Support`, so any test-level attempt at setting `XDG_DATA_HOME` did nothing. Fixed in #17 (commit `6f3f7a8`).

## What was kept

- 1 real `note_access` event: `arc-thinking-with-peiman`, source `note_get`, vault `vaultmind-identity`, timestamp `2026-04-21T22:27:31Z`.
- All other event types (344 `ask`, 274 `index_embed`, 193 `context_pack`, 133 `search`) are valid observations regardless of source, so they were left untouched.

## Why only `note_access`

Only `note_access` events feed the spreading-activation re-rank through `experiment.DB.AccessedNoteIDs()`. Other event types don't drive ranking, so test-derived ones there are harmless noise, not a signal-corruption risk.

## Script

[`scripts/purge-test-fixture-events.sql`](../../scripts/purge-test-fixture-events.sql) — idempotent; re-running removes zero rows.

Verified by running twice on a copy before touching prod. Backup taken before prod run:
`~/Library/Application Support/vaultmind/experiments.db.bak.20260422T220727Z`.

## After this cleanup

Once #18 (ask → note_access logging) is live — which it is, as of commit `2f211f9` — every real `vaultmind ask` produces a legitimate `note_access` event with source `"ask"`. Access history accumulates through normal use, and the `delta=0.2` measurement from #16 becomes meaningful to re-open.
