-- +goose Up
-- Migration 008: record whether an access DELIVERED note text.
--
-- Background: migration 007 added the caller dimension so `vaultmind self`
-- could separate the harness's footprint from the agent's engagement
-- footprint. That worked because caller was a sound PROXY for delivery —
-- the hook and neighbour callers genuinely never handed over any note
-- text, so excluding them excluded exactly the non-reads.
--
-- The delivery work (ask --excerpt, and the low-contrast body rule) made
-- both of those callers deliver. The proxy inverted overnight: `self` now
-- hides real reads, and hides more of them the better delivery works. On a
-- live 4,014-row ledger it reported 47 accessed notes — 87% excluded, a
-- growing share of them genuine.
--
-- Caller cannot be repaired into a delivery signal. resolveCaller collapses
-- anything with "hook" in VAULTMIND_CALLER to CallerHook, overriding the
-- explicit target/neighbour distinction the ask path passes, so for a
-- majority of rows the ledger cannot even tell the deliberate target from
-- the fan-out. The dimension is simply the wrong one for this question.
--
-- So record the fact directly, from the same AskResult.DeliveredTo value
-- the experiment ledger already writes — one source of truth for "did
-- content reach the agent", consulted by both ledgers.
--
-- NULL is deliberate for pre-existing rows, and is NOT the same as 0.
-- Those rows predate delivery tracking, and for them the caller heuristic
-- WAS correct: a hook row really did deliver nothing. Writing 0 would
-- assert a measurement nobody took; writing 1 would fabricate deliveries.
-- NULL says "unknown — fall back to what caller meant at the time", which
-- is the only honest reading of history.
--
-- caller is kept, and keeps its real meaning: WHO INITIATED the access.
-- That is still the right axis for "deliberate vs ambient"; it was only
-- ever wrong as a stand-in for what arrived.
ALTER TABLE note_accesses ADD COLUMN body_delivered INTEGER;

CREATE INDEX IF NOT EXISTS idx_note_accesses_body_delivered
    ON note_accesses(body_delivered);
