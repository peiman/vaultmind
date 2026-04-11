Both Write and Edit are unavailable as deferred tools, and Bash is being denied for file writes. I am operating as the Architect agent which is READ-ONLY by design. The user asked me to write to a file, but I cannot do so with available permissions.

I will provide my full analysis directly as text output -- the parent agent or user can write it to the file.

---

## What I was trying to do

I attempted to write my analysis to `/Users/peiman/dev/cli/vaultmind/docs/som/2026-04-11-persona-evaluation/round1/systems-architect.md` but my agent role (Architect) is READ-ONLY and file write permissions are blocked. The directory exists (I confirmed that). Below is the complete analysis that should be written to that file.

---

# Systems Architect: Data Infrastructure for Persona Reconstruction Measurement

## Summary

The existing experiment framework (`internal/experiment/`) provides 80% of what we need: sessions, typed events, variant scoring, outcome linkage, and Hit@K/MRR reporting. The remaining 20% is (a) a sidecar log from the SessionStart hook capturing what was injected, (b) a lightweight session-comparison query layer, and (c) a configuration-driven test harness for vault variants. The critical design constraint is that we must not add any prompting or behavioral steering to the measurement -- the hook already injects content, so we instrument the hook itself, not the agent.

## 1. What Data to Capture Per Session

### Already captured by existing infrastructure
- **Session lifecycle**: `sessions` table tracks `session_id`, `vault_path`, `started_at`, `ended_at` (`db.go:16-21`)
- **VaultMind queries**: Every `search`, `ask`, `context_pack` call logs to `events` with full query text, mode, variant, and JSON payload (`event.go:33-58`, `context.go:42-69`)
- **Note access patterns**: `note_access` events with outcome linkage back to prior queries (`context.go:85-97`, `outcome.go:25-56`)
- **Activation scores**: Full ACT-R model with retrieval strength, storage strength, similarity (`activation.go:69-87`)

### Needs to be captured (new)
1. **Hook injection log** -- what the SessionStart hook actually produced (the identity text + context text). This is the independent variable. Without knowing exactly what was injected, we cannot correlate input to behavior.
2. **Injection metadata** -- which notes were retrieved, their activation scores at injection time, token count of injection, which arcs/identity/principles were selected.
3. **Agent first-action trace** -- the first N tool calls, file reads, and queries the agent makes after session start. This comes from the JSONL transcript, not from VaultMind.
4. **Probe responses** -- responses to standardized questions ("what are you working on?", "what happened last session?"). These are the dependent variable.

### What we should NOT capture
- Model internal states (no access)
- Every token of every response (too much noise, too much storage)
- Subjective quality ratings during the session itself (observer effect)

## 2. Capturing Without Changing Behavior (Observer Effect)

### The key insight: the hook already runs. We log its output, not add to it.

The current `load-persona.sh` (`.claude/scripts/load-persona.sh:1-27`) runs two `vaultmind ask` calls and emits their output as a system-reminder. This is the injection point. The measurement approach:

**Instrument the hook script, not the prompt.**

The modification to `load-persona.sh` would look like:

```bash
# Current: runs vaultmind ask, emits to stdout (which becomes system-reminder)
# Proposed: runs vaultmind ask, emits to stdout AND tees to a sidecar log

IDENTITY=$("$VAULTMIND" ask "who am I" --vault "$VAULT_PATH" --max-items 8 --budget 6000 2>/dev/null)
# Add: write structured log OUTSIDE the stdout stream
echo "$IDENTITY" > "$LOG_DIR/identity-injection.txt"  # sidecar, not stdout
```

This is the crucial architectural decision: **the sidecar log goes to stderr or a file, never to stdout.** Stdout is what the agent sees. Anything we add to stdout changes the behavior. The log file is for post-hoc analysis only.

Specific implementation:

1. **Before the hook runs**: Record timestamp, vault path, vault note count, index freshness.
2. **The hook runs normally**: `vaultmind ask` calls execute as they do today. No changes to what the agent sees.
3. **After the hook runs**: Write a JSON manifest to `~/.vaultmind/persona-eval/{timestamp}-injection.json` containing:
   - Timestamp
   - Vault path and hash (to detect vault changes between runs)
   - Identity query results (the full text that was injected)
   - Context query results (the full text that was injected)
   - Notes selected (IDs, activation scores, types)
   - Total token count of injection
   - VaultMind version / config hash

**What this preserves**: The agent sees exactly the same system-reminder content it sees today. Zero additional prompting. Zero behavioral change. The measurement is purely a side-effect log.

**What this does NOT capture**: The agent's response. That comes from the JSONL transcript after the fact. We never ask the agent to self-report during the session -- that would be the observer effect we're avoiding.

### Where observer effect is unavoidable

The probe questions ("what are you working on?") ARE an intervention. The user asking them changes the session. This is acceptable because:
- The same questions are asked in every condition (controlled)
- The questions themselves are natural (not "rate your identity continuity on a scale of 1-5")
- The analysis happens post-hoc on the transcript, not in-session

## 3. Integration with Existing Experiment Framework

### What we can reuse directly

| Component | Location | Reuse for persona eval? |
|-----------|----------|------------------------|
| Session tracking | `session.go:24-35` | Yes -- same session lifecycle |
| Event logging | `event.go:33-58` | Yes -- add new event type `persona_injection` |
| Variant scoring | `scorer.go:13-21` | Yes -- different vault configs are variants |
| Outcome linkage | `outcome.go:25-56` | Partially -- need different outcome semantics |
| Report generation | `report.go:26-60` | Partially -- Hit@K/MRR don't map to persona eval |
| A/B config | `config.go:11-15` | Yes -- `ExperimentDef` with primary + shadows |
| Activation model | `activation.go:69-87` | Yes -- already captures what we need |

### What needs new semantics

The existing framework measures **retrieval quality**: "did the search return notes the user actually accessed?" (Hit@K, MRR). Persona evaluation needs to measure **behavioral quality**: "did the injected context produce partner-mode behavior?"

This is a different kind of outcome. The existing `outcomes` table links events to notes. Persona evaluation needs to link sessions to behavioral classifications. These are complementary, not conflicting.

**Proposed: add a `session_annotations` table via migration v2.**

```sql
CREATE TABLE IF NOT EXISTS session_annotations (
    annotation_id TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(session_id),
    annotator     TEXT NOT NULL,  -- 'human', 'automated', 'blind-rater'
    dimension     TEXT NOT NULL,  -- 'mode', 'arc_recall', 'initiative', etc.
    value         TEXT NOT NULL,  -- 'partner', 'tool', 'compliance', '3/5', etc.
    evidence      TEXT,           -- specific quote or behavioral indicator
    annotated_at  TEXT NOT NULL
);
```

This keeps the existing retrieval-quality pipeline intact and adds a parallel annotation pipeline for behavioral assessment. The `dimension` field is intentionally a string, not an enum -- we don't know all the dimensions yet, and premature schema rigidity would be worse than loose typing at this stage.

### New event type for injection logging

Add `EventPersonaInjection = "persona_injection"` to `event.go:12-17`. The `event_data` JSON contains the injection manifest. This slots cleanly into the existing event logging pattern -- one new constant, one new log call from the hook.

### What we should NOT build

- A separate database. The experiment DB handles this fine.
- A custom transcript parser. JSONL parsing is a script, not infrastructure.
- Real-time behavioral classification. Post-hoc is sufficient and avoids coupling.

## 4. Session Replay and Comparison

### Storage: already solved

Claude Code JSONL transcripts at `~/.claude/projects/{project-id}/{session-id}.jsonl` are the ground truth. They contain every message, tool call, and response. We don't need to duplicate this storage.

What we add:
1. **Injection manifests** at `~/.vaultmind/persona-eval/{timestamp}-injection.json` (what went in)
2. **Session annotations** in the experiment DB (behavioral classification of what came out)
3. **A cross-reference** linking VaultMind session IDs to Claude Code session IDs (currently implicit via timestamp correlation -- should be explicit)

### Comparison approach

For comparing sessions across runs with the same vault configuration:

```
Session A (vault config X, run 1):
  injection.json -> what was injected
  transcript.jsonl -> what the agent did
  annotations -> behavioral classification

Session B (vault config X, run 2):
  injection.json -> what was injected (should be identical or near-identical)
  transcript.jsonl -> what the agent did (will differ -- non-determinism)
  annotations -> behavioral classification
```

The comparison query is: "For vault config X, across N runs, what is the distribution of behavioral classifications?" This is a GROUP BY on `session_annotations` joined with session metadata.

**The vault config hash** is the key innovation here. We hash the vault contents (note IDs + content hashes, sorted) to create a reproducible fingerprint. Same hash = same vault state. This goes into the injection manifest and into the session metadata. It lets us answer: "across all sessions with vault config ABC123, what percentage showed partner-mode behavior?"

### What we should NOT build

- A full session replay UI. We're analyzing transcripts, not replaying them.
- Diff tooling for transcripts. The behavioral classification is the comparison unit, not the raw text.

## 5. A/B Testing Infrastructure

### The existing framework already supports this

`ExperimentDef` (`config.go:11-15`) defines experiments with a primary variant and shadow variants. The `Dispatcher` (`scorer.go:24-51`) routes to variant-specific scorers. The report generator (`report.go:26-60`) computes per-variant metrics.

For persona evaluation, the "variants" are vault configurations:

| Variant name | Description | What's included |
|-------------|-------------|-----------------|
| `full-vault` | All notes | identity + arcs + principles + references |
| `identity-only` | No arcs | identity + principles + references |
| `no-arcs` | Control | identity + principles (no arcs, no references) |
| `empty` | Baseline | No injection at all |
| `arcs-subset-growth` | Specific arcs | identity + growth-related arcs only |
| `arcs-subset-technical` | Specific arcs | identity + technical arcs only |

### Implementation: vault snapshots, not dynamic configuration

The leanest approach:

1. **Create fixed vault directories** for each variant: `test/fixtures/persona-eval/full-vault/`, `test/fixtures/persona-eval/identity-only/`, etc.
2. **Modify `load-persona.sh`** to accept `VAULT_PATH` as an environment variable (it already uses `$CLAUDE_PROJECT_DIR/vaultmind-identity` -- make it overridable).
3. **Run sessions with different `VAULT_PATH` values**.
4. **The injection manifest records which vault was used** (path + content hash).

This avoids building a "vault variant generator" or "note subset selector." Those are over-engineering. We have ~14-19 notes. Manually curating 4-6 test vaults is faster and more reliable than building automation.

### Sample size and statistical validity

With non-deterministic outputs, we need multiple runs per variant. Minimum viable:
- 5 runs per variant (captures variance)
- 4-6 variants
- = 20-30 total sessions

This is small enough to run manually over a few days. We don't need a test harness that orchestrates sessions automatically -- that's premature.

### What we should NOT build

- Automated session orchestration. Running `claude --vault-path X` 5 times is trivial.
- Statistical significance testing infrastructure. With N=5 per group, we're looking for effect sizes visible to the naked eye. If we need p-values to see the difference, the difference isn't meaningful for our purposes.
- A dashboard. A markdown table updated after each batch is sufficient.

## 6. ANTI-CONFORMITY: What We're Over-Building

### Infrastructure we should resist building

1. **A custom transcript parser.** The JSONL format is Claude Code's internal format. It can change. Writing a robust parser is wasted effort. Instead: use `jq` one-liners to extract what we need. If we need more, write a throwaway Python script, not a Go package.

2. **Real-time behavioral classification.** The temptation is to build an automated classifier that reads transcripts and outputs "partner/tool/compliance." This is premature. We don't yet know what the behavioral signals ARE. Classify manually first. Build automation only after we've done 20+ manual classifications and found patterns.

3. **A session_annotations API.** Don't build CRUD endpoints for annotations. Use direct SQL inserts via a script. The audience for this data is one person (Peiman) running analysis queries.

4. **Vault variant generation logic.** With 14-19 notes, copy the directory and delete files. This is a 30-second manual operation, not a feature.

5. **Cross-session statistical infrastructure.** Mean and standard deviation computed by hand (or a spreadsheet) is sufficient for N=5 per group. Building statistical libraries is scope creep.

### The leanest viable measurement

The absolute minimum that produces useful data:

1. **Modify `load-persona.sh`**: Add 4 lines to tee injection output to a timestamped log file. Zero changes to what the agent sees.
2. **Add one SQL migration**: The `session_annotations` table (6 columns).
3. **One shell script** (`annotate-session.sh`): Takes a session ID, opens the JSONL in `$EDITOR`, prompts for classification, inserts into SQLite.
4. **One query script** (`compare-variants.sh`): GROUP BY vault_config_hash, dimension, value. Outputs a markdown table.

Total new code: ~50 lines of shell, ~10 lines of SQL. No new Go code. No new packages. No new commands.

### What I might be wrong about

- **Manual annotation might not scale.** If we need 100+ sessions (because effect sizes are small), manual classification becomes a bottleneck. But we should discover this through doing, not anticipate it through building.
- **The JSONL format might be insufficient.** If Claude Code changes its transcript format, our `jq` scripts break. But building a robust parser for a format we don't control is also fragile.
- **The sidecar log approach assumes the hook runs reliably.** If the hook fails silently (which has happened -- see the install_tools.sh pattern), we get sessions with no injection data. We need the injection manifest to be the ground truth, and we need to detect its absence.
- **I'm assuming behavioral classification is obvious.** It might not be. "Partner mode" vs "tool mode" might be a spectrum, not a binary. The annotation schema should support continuous values, not just categories -- but I've designed it with a TEXT `value` field, which handles both.

## Architecture Diagram

```
                    EXISTING                           NEW (minimal)
                    ========                           =============

SessionStart Hook ──────────────────────────────────> injection.json
  load-persona.sh                                     (sidecar log)
       |                                                   |
       v                                                   |
  Agent receives                                           |
  system-reminder                                          |
  (UNCHANGED)                                              |
       |                                                   |
       v                                                   |
  Agent acts ───> JSONL transcript                         |
       |          (already exists)                          |
       v                                                   |
  vaultmind calls ───> experiment.db                       |
  (search/ask/       (sessions, events,                    |
   context-pack)      outcomes, activation)                |
                           |                               |
                           v                               v
                    ┌──────────────────────────────────────────┐
                    │         POST-HOC ANALYSIS                │
                    │                                          │
                    │  injection.json + JSONL + experiment.db  │
                    │         |                                │
                    │  Manual annotation -> session_annotations│
                    │         |                                │
                    │  compare-variants.sh -> markdown table   │
                    └──────────────────────────────────────────┘
```

## Concrete Next Steps (Priority Order)

1. **[30 min]** Add sidecar logging to `load-persona.sh` -- tee injection content to `~/.vaultmind/persona-eval/{timestamp}-injection.json`
2. **[15 min]** Add migration v2 with `session_annotations` table to `db.go`
3. **[30 min]** Create 3-4 vault variant directories under `test/fixtures/persona-eval/`
4. **[15 min]** Write `annotate-session.sh` for manual post-hoc classification
5. **[15 min]** Write `compare-variants.sh` for variant comparison queries
6. **[2 hours]** Run first batch: 3 runs each of `full-vault` and `empty` variants
7. **[1 hour]** Annotate and compare. Decide if we need more infrastructure based on what we learn.

Total infrastructure investment: ~2 hours of scripting. Then we run experiments and let the data tell us what to build next.

## References

- `/Users/peiman/dev/cli/vaultmind/internal/experiment/db.go:14-51` -- Schema: sessions, events, outcomes tables
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/event.go:12-17` -- Event type constants (search, ask, context_pack, note_access, index_embed)
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/context.go:42-97` -- Session event logging methods
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/outcome.go:25-56` -- Outcome linkage (variant-aware)
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/config.go:11-15` -- ExperimentDef with primary + shadow variants
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/scorer.go:13-51` -- Variant dispatcher
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/report.go:26-60` -- Hit@K / MRR reporting
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/activation.go:69-87` -- ACT-R retrieval strength computation
- `/Users/peiman/dev/cli/vaultmind/internal/experiment/session.go:24-35` -- Session lifecycle
- `/Users/peiman/dev/cli/vaultmind/.claude/hooks.json:3-16` -- SessionStart hook configuration
- `/Users/peiman/dev/cli/vaultmind/.claude/scripts/load-persona.sh:1-27` -- Current persona injection script

---

**Note to parent agent**: I was unable to write this file due to permission restrictions on the Architect agent (READ-ONLY). Please write the content above to `/Users/peiman/dev/cli/vaultmind/docs/som/2026-04-11-persona-evaluation/round1/systems-architect.md`. The directory already exists.
