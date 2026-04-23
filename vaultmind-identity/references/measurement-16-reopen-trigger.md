---
id: reference-measurement-16-reopen-trigger
type: reference
title: "When to Re-open #16 (Spreading Activation Measurement)"
created: 2026-04-23
vm_updated: 2026-04-23
tags:
  - reference
  - measurement
  - spreading-activation
  - trigger
related_ids:
  - arc-dogfood-rrf
  - arc-reading-symptoms
  - principle-measure-before-optimize
  - reference-current-context
---

# When to Re-open #16

GitHub issue [#16](https://github.com/peiman/vaultmind/issues/16) was closed on 2026-04-22 because its preconditions didn't hold: spreading activation silently no-ops on fresh data, the `ask` command wasn't logging `note_access`, and the production experiment DB had been corrupted by test-fixture leakage. See the [close comment](https://github.com/peiman/vaultmind/issues/16#issuecomment-4295814438) for the full trace.

#17, #18, and #19 fixed those preconditions. #16 is now blocked only on **accumulated real access history**.

## The trigger condition

Re-open #16 when this SQL against the production experiment DB returns ≥ 50:

```sql
SELECT COUNT(*)
FROM events
WHERE event_type = 'note_access'
  AND json_extract(event_data, '$.source') = 'ask'
  AND vault_path LIKE '%vaultmind-identity';
```

DB path: `~/Library/Application Support/vaultmind/experiments.db`

## Why 50

The SessionStart hook fires 2 asks per Claude Code boot. Normal ad-hoc queries during a session add more. 50 events is roughly 15-25 sessions of real use — enough that:

- The access log has covered most of the 17 identity-vault notes (so re-rank has real IDs to work with, not just 1).
- Access frequencies differ across notes (so `ComputeStorage` has a real distribution, not a flat one).
- The measurement can detect rank changes beyond single-note noise.

Going below 50 risks a null result that just reflects thin data. Going much above 50 means waiting longer than needed.

## Why this is in the vault, not a GitHub issue

A closed issue rots in the backlog. A vault reference note is loaded by the SessionStart hook whenever a relevant query surfaces it. When a future session asks "what's next for spreading activation?" or "should we re-open #16?", the retrieval surfaces this note and the trigger is right there, runnable.

This is Principle 9 (Automated Enforcement) applied to a waiting condition: encode the re-open trigger as something observable (a SQL query) rather than something to remember.

## What to do when the trigger fires

1. Run the SQL. Confirm ≥ 50.
2. Re-open #16 on GitHub (or open a fresh successor issue — preserving the original's post-mortem is fine).
3. Re-scope the hypothesis: the formula question (A/B/C from the old body) is settled — the shipped path is what gets measured. No interpretation ambiguity remains.
4. Run the comparison: condition A = hybrid only (no activation layer), condition B = shipped `query.Ask` with `AskConfig.ActivationFunc` wired to `computeActivationScores`. Primary metric: rank displacement on identity-bucket queries. Secondary: inversion count on technical queries.
5. Report honestly: if the result is "delta=0.2 doesn't help at N=50," that is the finding. Don't tune delta to make it look justified — that's the `arc-dogfood-rrf` mistake in a new shape.

## Related reading

- `arc-dogfood-rrf` — the last time a shipped retrieval claim turned out to be wrong against real data. Read before running the measurement.
- `arc-reading-symptoms` — the pattern of treating signals as conclusions. The measurement, when run, is a signal. The source of truth is still the code.
- `principle-measure-before-optimize` — the principle that this whole exercise exists to honor.
