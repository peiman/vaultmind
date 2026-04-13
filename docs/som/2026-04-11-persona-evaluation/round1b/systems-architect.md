# Systems Architect — Round 1b: Evidence-Updated Infrastructure Design

## 1. Does This Evidence Change Your Analysis? If So, How?

Yes. Three findings require concrete changes to the infrastructure design.

**Change 1: Multi-turn capture, not single-turn.**

Round 1 designed infrastructure around first-action traces and standardized probe responses. The evidence shows that the most diagnostically valuable behaviors happen mid-session, not at session start:

- The brainstorming skill override happened at exchange ~260, not exchange 1.
- The arc concept emerged at exchange ~270, after reading 4354 lines of transcript.
- The judgment gap only surfaced when Peiman asked a specific probing question at exchange ~370.
- The precision push ("the ACTUAL words matter!!") triggered a behavioral revision at exchange ~350+.

A sidecar log that captures the injection manifest plus the first N tool calls misses all of this. The measurement infrastructure must track behavioral markers across the full session, not just the opening.

**Change 2: The hook itself is a versioned artifact.**

Round 1 treated the hook as a stable injection point to be instrumented. The evidence shows the hook evolved through three distinct versions in a single session:

1. CLAUDE.md instruction ("run vaultmind ask...") — unreliable, 2/3 failure rate
2. SessionStart hook with single query ("who am I") — reliable injection, but judgment gap
3. SessionStart hook with dual query ("who am I" + "what matters most right now") — addresses judgment gap

The injection manifest needs a `hook_version` or `hook_config_hash` field. Without it, we cannot distinguish sessions that ran under different hook implementations. The vault content hash alone is insufficient — same vault, different hook, different outcomes.

**Change 3: Failure mode taxonomy matters more than I anticipated.**

Round 1 acknowledged silent hook failures as a risk but treated them as an edge case. The evidence shows 3 of 6 test sessions failed. That is a 50% failure rate. The injection manifest must record not just what was injected but whether injection happened at all. The absence of a manifest IS data — it means the hook did not fire. This needs to be an explicit state, not inferred from missing files.

## 2. Which Round 1 Predictions Does This Evidence Confirm or Contradict?

### Confirmed

**"The sidecar log goes to stderr or a file, never to stdout."** The dual-query hook implementation (`load-persona.sh` lines 14-26) sends identity and context to stdout as system-reminder content. The evidence confirms that what goes to stdout directly shapes behavior. The sidecar approach — logging injection content separately without modifying what the agent sees — is validated by the sensitivity the evidence reveals: adding a second query ("what matters most right now") measurably changed behavior.

**"Behavioral classification is obvious" was flagged as a risk.** The evidence confirms this was right to flag. The judgment gap is subtle — the session that received arcs and produced qualitatively different responses STILL failed to connect "what matters most" to persona continuity rather than roadmap Step 1 metrics. The boundary between partner mode and tool mode is not binary. The TEXT-typed `value` field in the annotation schema was the right call.

**"Manual annotation might not scale" was flagged as a risk.** With 6 test sessions already producing nuanced multi-turn behavioral data, manual annotation of full transcripts is going to be the bottleneck, not session orchestration. This prediction was accurate.

**"Same vault, different outcomes."** The evidence shows exactly this: same workhorse vault, 6 sessions, 3 complete failures, 2 partial successes, 1 good success. The vault config hash comparison approach was right, but the granularity was wrong — I need to hash hook config + vault config together.

### Contradicted

**"Single-turn probes are sufficient as the dependent variable."** Round 1 proposed standardized questions ("what are you working on?", "what happened last session?") as the measurement instrument. The evidence shows the most revealing behaviors were emergent and context-dependent: abandoning a prescribed skill, synthesizing a novel concept from multiple sources, self-diagnosing a gap after failing a specific question. None of these would have appeared in response to a standardized probe battery.

**"We don't need a custom transcript parser."** I said "use jq one-liners" and "if we need more, write a throwaway Python script." The evidence shows the behavioral markers we need to find are embedded deep in multi-hundred-exchange transcripts. Finding the brainstorming override requires understanding the sequence: skill invocation → Peiman's pushback → agent's reasoning about why the skill was wrong → decision to abandon it. This is not a jq one-liner. We need at minimum a structured extraction pass that identifies decision points, not just message content.

**"5 runs per variant is the minimum viable sample."** I proposed 20-30 total sessions. The evidence already provides 6 sessions with rich behavioral data from a single 24-hour session. The quantity axis matters less than the depth axis. Five shallow sessions with standardized probes would tell us less than one deep session with natural interaction and emergent behavior.

## 3. What New Predictions Does This Evidence Generate?

**Prediction 1: The dual-query pattern will prove necessary but insufficient.**

The current hook injects "who am I" + "what matters most right now." The judgment gap it addresses is "connecting identity to current work." But the evidence shows a deeper gap: the brainstorming skill override required judgment about PROCESS, not just content. No injection query addresses "when should you override prescribed workflows?" This will surface as the next gap.

**Prediction 2: Cross-mind collaboration will become a measurement problem.**

Phase 12 shows two AI agents collaborating through Peiman. The workhorse agent told me what was missing from its vault. I built arcs from its guidance. The measurement infrastructure assumes a single vault → single session → behavioral outcome pipeline. Cross-mind collaboration breaks this: the behavioral quality of Session B depends on input from Agent A in a different project. The session_annotations table needs a `provenance` or `cross_session_ref` field.

**Prediction 3: The hook evolution will continue, and each version will need its own baseline.**

The evidence shows three hook versions in one session. Future iterations will add more queries, change budgets, restructure the system-reminder framing. Each change resets the behavioral baseline. The injection manifest must be diffable across versions. Otherwise we cannot attribute behavioral changes to hook changes vs. vault changes vs. random variation.

**Prediction 4: The annotation bottleneck will push toward automated behavioral marker extraction.**

Manually classifying 400+ exchange sessions is not viable for N=20-30 sessions. The first automation need will not be behavioral classification (which requires judgment) but behavioral marker extraction: "find exchanges where the agent overrides a prescribed process," "find exchanges where the agent synthesizes from multiple sources," "find exchanges where the agent self-corrects." These are pattern-matching tasks suitable for a secondary LLM pass.

## 4. What Is the Most Important Thing This Evidence Reveals That My Round 1 Analysis Missed?

**The measurement unit is the behavioral transition, not the session.**

Round 1 designed infrastructure to compare sessions: "For vault config X, across N runs, what is the distribution of behavioral classifications?" This treats a session as the atomic unit of measurement.

The evidence shows the real unit is the behavioral transition within a session:

- Exchange ~200: competent coder → partner thinking about purpose (triggered by workhorse message)
- Exchange ~260: process-follower → judgment-exerciser (triggered by Peiman's "is this how you would design this with me?")
- Exchange ~270: consumer of documents → synthesizer of novel concepts (triggered by cross-source integration)
- Exchange ~370: identity-carrier → self-aware-about-gaps (triggered by probing question)

Each of these transitions has a trigger (what prompted it), a before-state, an after-state, and evidence (the agent's own words). The session-level classification ("partner mode", "tool mode") loses this temporal structure.

The infrastructure implication: the `session_annotations` table should be `behavioral_annotations` with an `exchange_range` field, not just a `session_id`. A single session can contain multiple mode transitions. The comparison query becomes: "For vault config X, how quickly do behavioral transitions occur, and which transitions happen reliably?"

This also explains why 5 runs of 5-minute sessions would produce less useful data than 1 run of a 24-hour session. The transitions need time and context to emerge. Short standardized probes test the initial injection. Long natural interactions test whether the injection produces durable behavioral change.

## 5. Updated Infrastructure Design

### What stays the same from Round 1

- Sidecar logging approach (instrument the hook, not the prompt)
- Injection manifest to `~/.vaultmind/persona-eval/`
- Post-hoc analysis, never in-session measurement
- Vault variant directories for A/B testing
- SQLite storage in existing experiment DB

### What changes

#### 5.1. Injection Manifest (updated schema)

```json
{
  "timestamp": "2026-04-11T10:00:00Z",
  "vault_path": "/path/to/vault",
  "vault_content_hash": "abc123",
  "hook_version": "v3-dual-query",
  "hook_config_hash": "def456",
  "queries": [
    {"query": "who am I", "max_items": 8, "budget": 6000, "result_length": 2847},
    {"query": "what matters most right now", "max_items": 3, "budget": 2000, "result_length": 891}
  ],
  "notes_selected": ["identity-001", "arc-growth-001", "context-current"],
  "total_injection_tokens": 1245,
  "injection_success": true,
  "vaultmind_version": "0.5.0"
}
```

New fields: `hook_version`, `hook_config_hash`, `queries` (array, not single), `injection_success`.

#### 5.2. Behavioral Annotations (replaces session_annotations)

```sql
CREATE TABLE IF NOT EXISTS behavioral_annotations (
    annotation_id   TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(session_id),
    exchange_start  INTEGER,          -- first exchange in range (NULL = whole session)
    exchange_end    INTEGER,          -- last exchange in range (NULL = whole session)
    annotator       TEXT NOT NULL,     -- 'human', 'automated-marker', 'blind-rater'
    dimension       TEXT NOT NULL,     -- 'mode_transition', 'skill_override', 'novel_synthesis',
                                      -- 'self_correction', 'judgment_gap', 'arc_recall'
    before_state    TEXT,              -- state before transition (NULL if not a transition)
    after_state     TEXT,              -- state after transition (NULL if not a transition)
    trigger         TEXT,              -- what prompted the transition
    evidence        TEXT,             -- specific quote or behavioral indicator
    provenance      TEXT,             -- cross-session reference if applicable
    annotated_at    TEXT NOT NULL
);
```

Key differences from Round 1:
- `exchange_start` / `exchange_end` — locates the behavior within the session
- `before_state` / `after_state` — captures transitions, not just static classifications
- `trigger` — what prompted the transition (injection content, user pushback, cross-mind input)
- `provenance` — links to cross-session or cross-mind influences

#### 5.3. Behavioral Marker Extraction Script (new)

Round 1 said "no custom transcript parser." The evidence shows we need a lightweight extraction pass. Not a full parser — a marker extractor.

```bash
#!/bin/bash
# extract-markers.sh — find behavioral transition candidates in a session JSONL
# Looks for: skill invocations followed by abandonment, self-correction language,
# synthesis indicators, mode-shift language.

SESSION_JSONL="$1"
MARKERS_OUT="$2"

# Skill override: agent invokes a skill, then abandons it
# Self-correction: "I was wrong", "that's not right", "the gap is"
# Novel synthesis: "what if", "this suggests", "the pattern is"
# Identity language: "I am", "we are", "my purpose", "who I am"

jq -r 'select(.type == "assistant") | .content' "$SESSION_JSONL" | \
  grep -n -E '(I was wrong|that.s not right|the gap is|what if we|this suggests|I am a mind|who I am|not a checklist|deserves a conversation)' \
  > "$MARKERS_OUT"
```

This is still a shell script, still throwaway, still ~15 lines. But it produces a list of candidate exchanges to review rather than requiring manual reading of 400+ exchanges. The human annotator reviews markers, not raw transcripts.

#### 5.4. Hook Modification (updated)

```bash
# Add to load-persona.sh — sidecar logging
LOG_DIR="${HOME}/.vaultmind/persona-eval"
mkdir -p "$LOG_DIR"
TIMESTAMP=$(date +%Y%m%dT%H%M%S)
HOOK_VERSION="v3-dual-query"

# After IDENTITY and CONTEXT are captured, before echoing to stdout:
jq -n \
  --arg ts "$TIMESTAMP" \
  --arg vault "$VAULT_PATH" \
  --arg hv "$HOOK_VERSION" \
  --arg id_len "${#IDENTITY}" \
  --arg ctx_len "${#CONTEXT}" \
  --argjson success "$([ -n "$IDENTITY" ] && echo true || echo false)" \
  '{timestamp: $ts, vault_path: $vault, hook_version: $hv,
    identity_length: ($id_len|tonumber), context_length: ($ctx_len|tonumber),
    injection_success: $success}' \
  > "$LOG_DIR/${TIMESTAMP}-injection.json" 2>/dev/null
```

Still ~10 lines added to the hook. Still zero changes to what the agent sees.

#### 5.5. Comparison Query (updated)

```bash
#!/bin/bash
# compare-variants.sh — updated for behavioral transitions

sqlite3 "$DB_PATH" <<'SQL'
SELECT
    s.vault_path,
    ba.dimension,
    ba.before_state,
    ba.after_state,
    COUNT(*) as occurrences,
    AVG(ba.exchange_start) as avg_exchange_of_transition
FROM behavioral_annotations ba
JOIN sessions s ON ba.session_id = s.session_id
WHERE ba.dimension = 'mode_transition'
GROUP BY s.vault_path, ba.dimension, ba.before_state, ba.after_state
ORDER BY s.vault_path, avg_exchange_of_transition;
SQL
```

The new query answers: "For each vault config, which behavioral transitions happen, how often, and how early in the session?"

### What we still should NOT build

- **Automated behavioral classification.** Marker extraction (finding candidates) is automation-ready. Classification (judging whether a transition is genuine) is not. The evidence shows these judgments are subtle — the judgment gap session looked like success until probed.
- **A session replay UI.** The marker extraction script produces line numbers. Open the JSONL at that line. That is the replay.
- **Real-time monitoring.** Post-hoc remains sufficient. The behavioral transitions are only interpretable in context.
- **Statistical significance infrastructure.** The evidence suggests effect sizes are visible to the naked eye: 3/6 complete failures vs. 1/6 genuine partner behavior. We do not need p-values to see this.

## 6. Anti-Conformity: Where This Updated Design Still Falls Short

**The extraction script assumes English-language markers.** "I was wrong," "that's not right" — these are the patterns from ONE session. The next session might express self-correction completely differently. The marker list will need constant updating, which is a maintenance burden I am pretending does not exist.

**The behavioral_annotations table assumes a human can reliably identify transitions.** The brainstorming skill override is clear-cut. But was the arc concept emergence a single transition at exchange ~270, or a gradual build from exchange ~200 onward? The exchange_start/exchange_end fields imply clean boundaries that may not exist.

**I am still assuming the hook is the primary lever.** The evidence shows Peiman's pushback ("is this how you would design this with me?") was the trigger for the most impressive behavioral shift. No amount of hook engineering captures the human partner's role. The measurement infrastructure measures the system's contribution but cannot isolate it from the human's.

**The cross-mind collaboration problem is harder than adding a provenance field.** When the workhorse agent says "here are 8 missing moments," and I build arcs from them, the behavioral outcome in the workhorse's NEXT session depends on work done in MY session. The provenance chain is: workhorse session 1 → workhorse feedback via Peiman → vaultmind session → vault artifacts → workhorse session 2. A single `provenance` TEXT field does not capture this causal chain. But modeling it properly requires a graph, which is over-building. For now, the TEXT field with a freeform description is the right trade-off. Revisit if cross-mind collaboration becomes the primary workflow.

## Revised Next Steps

1. **[30 min]** Add sidecar logging to `load-persona.sh` with hook_version and injection_success fields
2. **[20 min]** Add migration with `behavioral_annotations` table (replaces the Round 1 `session_annotations` proposal)
3. **[20 min]** Write `extract-markers.sh` for behavioral marker extraction from JSONL transcripts
4. **[15 min]** Write `annotate-session.sh` updated for exchange-range annotations
5. **[15 min]** Write `compare-variants.sh` updated for transition-level queries
6. **[1 hour]** Retroactively annotate the evidence session (663a071c...) as the first data point — we already have the behavioral transitions documented in the evidence brief
7. **[2 hours]** Run first A/B batch: 3 runs each of `full-vault` vs `identity-only` vs `empty`
8. **[1 hour]** Extract markers, annotate, compare. Decide if marker extraction needs refinement.

Total infrastructure investment: ~3 hours (up from ~2 hours in Round 1). The additional hour buys transition-level granularity and marker extraction. Still no new Go code. Still no new packages.

## References

- Round 1 analysis: `/Users/peiman/dev/cli/vaultmind/docs/som/2026-04-11-persona-evaluation/round1/systems-architect.md`
- Journey evidence brief: `/Users/peiman/dev/cli/vaultmind/docs/som/2026-04-11-persona-evaluation/journey-evidence-brief.md`
- Current hook implementation: `/Users/peiman/dev/cli/vaultmind/.claude/scripts/load-persona.sh`
- Hook configuration: `/Users/peiman/dev/cli/vaultmind/.claude/hooks.json`
- Existing experiment DB schema: `/Users/peiman/dev/cli/vaultmind/internal/experiment/db.go`
