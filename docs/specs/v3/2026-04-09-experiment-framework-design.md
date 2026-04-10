# VaultMind Experiment Framework — Design Spec

**Date:** 2026-04-09
**Goal:** General-purpose experiment instrumentation for VaultMind that enables data-driven feature improvement through shadow scoring, outcome tracking, and multi-tier telemetry.

## Vision

Every VaultMind instance instruments its operations — searches, context-packs, note accesses — as structured experiment events. Shadow variants compute alternative scorings alongside the primary (shown) result. Outcome tracking links "what was shown" to "what was actually used." Over time, this data drives parameter tuning, scoring model improvements, and new feature discovery.

Phase 1: Local experiment DB per user. Phase 2: Centralized collection via opt-in telemetry, feeding into daana.dev information models and automated improvement pipelines.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Experiment DB location | User-global `~/.vaultmind/experiments.db` | One DB across all vaults, natural unit for Phase 2 collection |
| Shadow scoring | Multiple variants computed simultaneously, one shown | Enables offline A/B testing without affecting UX |
| Outcome signal | `note_access` events (note get, recall, context-pack delivery) | Maps to Hebbian "synapse fired" — the note was actually used |
| Telemetry tiers | Off / Anonymous / Full | Anonymous default after opt-in prompt, full for early adopters |
| Full tier includes vault | Yes — vault index DB snapshot on sync | Enables causal analysis (why did this ranking work/fail?) |
| Event schema | Feature values baked into events at log time | Experiment DB is self-contained, no vault dependency for analysis |

## Architecture

```
vaultmind search/ask/context-pack
    |
    v
Experiment Framework
    |-- Run primary variant (shown to user)
    |-- Run shadow variants (computed, not shown)
    |-- Log all variant results with feature values
    |-- Return primary result to user
    |
    v
~/.vaultmind/experiments.db
    |-- events table (all variant results per operation)
    |-- sessions table (start/end timestamps)
    |-- outcomes table (note accesses linked to prior events)
    |
    v (Phase 2, opt-in)
Centralized DB --> daana.dev pipelines --> improved defaults
```

## Component 1: Session Tracking

Every VaultMind invocation is a session.

```sql
CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY,
    vault_path TEXT NOT NULL,
    started_at TEXT NOT NULL,  -- RFC3339
    ended_at   TEXT            -- NULL until session ends
);
```

On startup: generate UUID session ID, insert with `started_at`. On exit (via defer): update `ended_at`. This provides the active/idle time partitioning needed for compressed idle time activation scoring.

**Crash safety:** If the process is killed before `ended_at` is set, the session row has `ended_at = NULL`. On next startup, detect orphaned sessions and set their `ended_at` to their last event timestamp (or `started_at + 1 minute` if no events).

**Outcome window:** Configurable via `experiments.outcome_window_sessions` (default: 2). An outcome is linked to events from the current session plus the N-1 previous sessions. This controls how far back we look for "what was the user responding to."

## Component 2: Event Logging

Each instrumented command logs a structured event.

```sql
CREATE TABLE events (
    event_id    TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(session_id),
    event_type  TEXT NOT NULL,  -- search, ask, context_pack, note_access, index_embed
    timestamp   TEXT NOT NULL,  -- RFC3339
    vault_path  TEXT NOT NULL,
    query_text  TEXT,           -- for search/ask events
    query_mode  TEXT,           -- keyword, semantic, hybrid
    primary_variant TEXT,       -- which variant was shown
    event_data  TEXT NOT NULL   -- JSON: variant results, scores, metadata
);
```

The `event_data` JSON contains all variant results with per-note feature values:

```json
{
  "primary_variant": "compressed-0.2",
  "variants": {
    "compressed-0.2": {
      "results": [
        {
          "note_id": "concept-spreading-activation",
          "rank": 1,
          "features": {
            "retrieval_strength": 1.2,
            "storage_strength": 2.4,
            "cosine_similarity": 0.78,
            "sparse_dot_product": 0.45,
            "colbert_maxsim": 3.2,
            "graph_distance": 1,
            "fts_score": 0.92,
            "rrf_score": 0.032
          }
        }
      ]
    },
    "wall-clock": { "results": [...] },
    "none": { "results": [...] }
  }
}
```

For anonymous telemetry, `note_id` and `query_text` are stripped before sync.

## Component 3: Outcome Tracking

Note access events serve as the outcome signal.

```sql
CREATE TABLE outcomes (
    outcome_id  TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL REFERENCES events(event_id),
    note_id     TEXT NOT NULL,
    variant     TEXT NOT NULL,  -- which variant predicted this note
    rank        INT NOT NULL,   -- what rank the variant gave it
    accessed_at TEXT NOT NULL,
    session_id  TEXT NOT NULL
);
```

When a `note_access` event occurs (note get, memory recall, context-pack content served), the framework:
1. Looks back at recent search/ask/context_pack events (same session + previous session)
2. For each variant in those events, checks if the accessed note appeared in the variant's results
3. If yes, inserts an outcome record linking the event, variant, rank, and access

This produces the hit-rate data needed for evaluation: "variant X predicted this note at rank 3, and it was indeed accessed."

## Component 4: Experiment Configuration

```yaml
# ~/.vaultmind/config.yaml
telemetry: anonymous  # off | anonymous | full

experiments:
  activation:
    enabled: true
    primary: compressed-0.2
    shadows:
      - wall-clock
      - compressed-0.5
      - none
  retrieval_mode:
    enabled: true
    primary: hybrid
    shadows:
      - keyword
      - semantic
```

Experiments are defined by name. Each has a primary variant (shown) and shadow variants (computed, not shown). The framework dispatches to the appropriate scorer for each variant.

## Component 5: Telemetry Opt-In

On first run (no `~/.vaultmind/config.yaml` exists), prompt:

```
Help improve VaultMind?
  [1] Anonymous usage statistics (recommended)
  [2] Full data sharing including queries and vault content (for early adopters)
  [3] No data collection

Choice [1]:
```

Mapping:
- 1 = `telemetry: anonymous` — feature values only, no IDs or text
- 2 = `telemetry: full` — complete events + vault index DB snapshot on sync
- 3 = `telemetry: off` — no sync (local experiments still run)

Changeable: `vaultmind config set telemetry off`

## Component 6: Instrumented Commands

| Command | Event Type | Logged Data |
|---------|-----------|-------------|
| `search` | `search` | Query, mode, per-variant ranked results with feature values |
| `ask` | `ask` | Query, per-variant search results + context-pack contents |
| `note get` | `note_access` | Note ID, triggers outcome linkage |
| `memory recall` | `note_access` | Note ID, triggers outcome linkage |
| `memory context-pack` | `context_pack` | Seed note, per-variant packed lists with feature values |
| `index --embed` | `index_embed` | Model, duration, note count, error count |

## Component 7: Analysis (Phase 1)

`vaultmind experiment report` — CLI command reading the local experiment DB:

```
$ vaultmind experiment report --experiment activation

Experiment: activation (47 sessions, 312 events, 89 outcomes)

Variant               Hit@5   Hit@10  MRR     Events
compressed-0.2 (P)    0.42    0.58    0.51    312
compressed-0.5        0.38    0.55    0.47    312
wall-clock            0.31    0.48    0.39    312
none                  0.35    0.52    0.43    312

(P) = primary variant shown to user
Best performer: compressed-0.2 (hit@5: 0.42)
```

Metrics:
- **Hit@K**: fraction of events where at least one of the variant's top-K results was accessed in the outcome window
- **MRR** (Mean Reciprocal Rank): average of 1/rank for the first accessed result per event

## Implementation Strategy

### Experiment DB Migrations

The experiment DB (`~/.vaultmind/experiments.db`) uses its own schema versioning — NOT goose (which manages the vault index DB). On open, check a `schema_version` pragma or `meta` table, apply migrations sequentially. Simple versioned SQL since the schema is small and stable.

### What to build now (Phase 1)

1. Experiment DB schema + migrations (sessions, events, outcomes tables)
2. Session tracking (open/close on each invocation)
3. Event logging infrastructure (generic, any command can emit events)
4. Outcome linkage (link note_access to recent events)
5. Instrument existing commands: search, ask, context-pack, note get
6. Experiment config parsing
7. Shadow scoring dispatcher (run N variants, return primary)
8. `experiment report` command
9. Telemetry opt-in prompt

### What to build later (Phase 2)

- Collection sync to centralized DB
- Vault snapshot export for full-tier telemetry
- daana.dev pipeline integration
- Parameter sweep / empirical fitting tools

### First experiment: Activation Scoring

Built on top of the framework:
- Compressed idle time activation computation using session timestamps
- Dual-strength scoring (retrieval + storage)
- Activation boost in context-pack assembly
- Four variants: compressed-0.2 (primary), compressed-0.5, wall-clock, none

## Testing Strategy

- Session tracking: verify open/close, verify `ended_at` set on clean exit
- Event logging: verify schema, verify JSON serialization, verify anonymous stripping
- Outcome linkage: mock events + note_access, verify correct linkage
- Shadow scoring: mock scorers, verify all variants computed and primary returned
- Report: seed experiment DB, verify metric computation
- Telemetry prompt: verify config persistence
