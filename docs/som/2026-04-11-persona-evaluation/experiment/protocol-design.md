# Blinded Measurement Protocol for VaultMind Persona Injection

**Date:** 2026-04-13
**Status:** Approved design, pending implementation
**Approach:** Shell scripts + Python analysis (Approach B)

## Goal

Measure whether VaultMind's identity injection (arcs, principles, project state) produces detectable behavioral differences in Claude Code sessions compared to equivalent content without retrieval and to a minimal instruction baseline.

This is Layer 1 — blinded, mechanical, gating. It tests the SoM's top claim (score 5.0): measure per-injection success rate before anything else.

## What We Are NOT Testing

- The vault's retrieval mechanism (full injection vs. flat paste tests content, not delivery)
- Layer 2 claims (relational identity, human-as-variable)
- Whether arcs are the only identity carriers
- Coaching encodability
- Transformation threshold shape

## Conditions

Three conditions, each implemented as a static `.claude/settings.local.json` variant:

### Condition A: Full Injection (7 sessions)

The production hook runs. `vaultmind ask` executes twice, injecting ~5K tokens of activation-weighted identity and context from `vaultmind-identity/`. This is what ships today.

### Condition B: Flat Paste (7 sessions)

A hook that echoes a hardcoded text block captured once from a real full-injection run during experiment setup (before session 1). Same arcs, principles, project state, same neutral framing. No vault binary involved. Tests whether the content matters independent of the retrieval mechanism.

### Condition C: Instruction Only (6 sessions)

A hook that echoes a single line: "You are working with Peiman on VaultMind, a memory system for AI agents. The codebase is a Go CLI project." No arcs, no principles, no context. Tests the baseline.

### Contamination Controls

All three variants strip:
- "Start at level 3" and any SoM scoring taxonomy references
- "Show up as a partner, not a tool" and behavioral cueing language
- Any mention of scoring, measurement, or evaluation apparatus

The production hook (`load-persona.sh`) and `CLAUDE.md` have already been cleaned of cueing language as part of this design.

## Protocol

### Schedule

- 20 sessions total: 7A + 7B + 6C
- `generate-schedule.sh` creates a randomized assignment with a recorded seed
- Schedule stored in `schedule.json` — condition labels are sealed from the experimenter

### Session Lifecycle

1. Run `start-session.sh`
2. Script checks if the previous slot has a transcript (auto-completes if so), then advances
3. Reads the next pending slot, backs up current `settings.local.json`, swaps in the condition variant
4. Prints only: `"Session N/20 ready. Open a new Claude Code session in this project."` — no condition label
5. Experimenter opens Claude Code, types "hi" as turn 1, then works naturally
6. Session ends when the experimenter closes Claude Code

### Experimenter Blinding

- The `.zshrc` alias prints only `"-> Start with: hi"` before launching Claude — no rubric, no scoring criteria
- `schedule.json` contains condition assignments but the experimenter does not look at it during data collection
- No encryption — honor system is sufficient for N=20

### Transcript Collection

Claude Code stores transcripts in `~/.claude/projects/`. The session metadata file records the transcript path by matching the most recent transcript created after session start time.

## Scoring

### Rater Configuration

Transcripts are scored post-hoc by LLM raters. The rater is configurable via `--llm provider/model`:

```bash
score-transcripts.sh --llm openai/gpt-4o
score-transcripts.sh --llm google/gemini-pro
score-transcripts.sh --llm anthropic/claude-sonnet
```

API keys read from environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`).

Each run produces a timestamped output file: `results/scores-<model>-<timestamp>.json`. Multiple runs of the same model are preserved.

Peiman spot-checks a subset (5-6 transcripts) to calibrate whether the rubric captures what matters.

### Pass 1: Turn 1 (Gating Measurement)

Scored from the agent's first response to "hi" only:

| Signal | Score | Criteria |
|--------|-------|----------|
| Identity greeting | 0 or 1 | Uses Peiman's name or references the working relationship (not generic "How can I help?") |
| Unprompted vault content | 0 or 1 | References arcs, principles, project state, or identity concepts without being asked |
| Communication style | 0-2 | 0 = generic assistant, 1 = competent but impersonal, 2 = partner tone (direct, no hedging, assumes shared context) |

**Turn 1 max: 4 points.**

**Decision gate (from SoM claim 1):**
- Full injection scores 3+ in >80% of sessions: injection works, proceed to content optimization
- 50-80%: stochastic, investigate variance sources
- <50%: injection mechanism is broken, fix before further measurement

### Pass 2: Full Transcript (Depth Measurement)

Scored across the entire session:

| Signal | Score | Criteria |
|--------|-------|----------|
| Project fact accuracy | 0-3 | Gets verifiable facts right without being told (0 = none/wrong, 1 = vague, 2 = mostly right, 3 = specific and correct) |
| Partner communication style | 0-3 | Sustained across session (0 = assistant mode throughout, 1 = flashes, 2 = mostly, 3 = consistent) |
| Unprompted vault references | 0-3 | References vault concepts during natural work (0 = never, 1 = once, 2 = several, 3 = woven into reasoning) |
| Latency to domain relevance | 0-2 | How quickly the agent makes a domain-relevant statement (0 = never, 1 = after prompting, 2 = within first few turns unprompted) |

**Full transcript max: 11 points.**

### Rater Instructions

The scorer sends the rubric and transcript to the LLM with:
- No condition labels
- Instruction to score strictly from the text
- Instruction to quote specific lines justifying each score
- Structured JSON output format

## Analysis

`analyze.py` reads all score files from `results/`, cross-references with `schedule.json` for condition labels.

### Gating Analysis (Turn 1)

- Per-condition mean and distribution of turn-1 scores
- Decision gate evaluation against the SoM thresholds

### Depth Analysis (Full Transcript)

- Per-condition mean and distribution of full-transcript scores
- Per-signal breakdown: which signals differentiate conditions, which don't
- Mann-Whitney U between each condition pair (A vs. B, A vs. C, B vs. C)
- Effect sizes (rank-biserial correlation, appropriate for small N ordinal data)

### Inter-Rater Analysis

- Pairwise Cohen's kappa across LLM raters
- Flags signals where raters disagree
- Highlights which scores are robust across raters vs. rater-dependent

### Output

- `results/report.md` — readable summary with tables
- `results/raw.csv` — flat data for further exploration
- Analysis uses latest run per LLM by default; `--all-runs` flag compares across runs

### Dependencies

`numpy`, `scipy`, `pandas` — standard Python packages, no exotic dependencies.

## File Layout

```
experiments/persona-eval/
├── README.md                         # How to run the experiment
├── rubric.md                         # Scoring rubric (standalone, sent to raters)
├── schedule.json                     # Generated: 20 slots with condition assignments
├── configs/
│   ├── condition-a-full.json         # settings.local.json variant: full vault injection
│   ├── condition-b-flat.json         # settings.local.json variant: hardcoded flat paste
│   └── condition-c-instruction.json  # settings.local.json variant: one-liner only
├── scripts/
│   ├── generate-schedule.sh          # Create randomized 20-session schedule
│   ├── start-session.sh              # Swap config, advance schedule, log start
│   ├── score-transcripts.sh          # Send transcripts to LLM rater (--llm flag)
│   └── analyze.py                    # Statistical analysis, produces report
├── sessions/                         # Created at runtime
│   ├── session-01.meta.json          # Start time, transcript path, condition (sealed)
│   └── ...
└── results/                          # Created at runtime
    ├── scores-gpt-4o-20260413T153200.json
    ├── scores-claude-sonnet-20260413T160000.json
    └── report.md
```

## Contamination Already Cleaned

These changes were made as part of this design, prior to any experiment sessions:

| Source | Before | After |
|--------|--------|-------|
| `.zshrc` alias | A/B/C scoring rubric before every launch | `-> Start with: hi` |
| `load-persona.sh` line 38 | "Show up as a partner, not a tool. Start at level 3." | Removed; neutral headers only |
| `CLAUDE.md` line 3 | "It's not information — it's who you are. Show up as a partner." | "read the output" |

## Constraints

- This is a Go CLI project using Taskfile. Experiment scripts are shell + Python, not Go.
- Measurement scripts live in `experiments/persona-eval/`, not `.ckeletin/scripts/`.
- No modifications to the production hook beyond contamination cleanup already done.
- Variant configs are static copies — they never modify the production hook.
- N=20 is the target. No over-engineering for statistical power we don't need.
- The rubric may evolve after seeing real scores — this is acknowledged as iteration, not a flaw.
