# VaultMind

**Associative memory for AI agents — persona reconstruction and semantic retrieval over Git-backed Obsidian vaults.**

VaultMind is built **for a human collaborating with an AI agent**: the agent reads the vault as long-term memory, both the human and the agent curate the markdown. It's research-grade and actively dogfooded — pre-v0.1.0, no release binaries cut yet.

---

## The problem

AI agents start each session from zero. They have strong parametric knowledge but no autobiographical memory, no arcs of growth, no continuity of identity across conversations. The mind that traced 140,000 lines of code yesterday is the same mind facing a blank prompt today.

VaultMind closes that gap. A vault of markdown notes becomes queryable, activation-weighted memory that an agent can reconstruct itself from at session start — and continue from, not start from.

## Get started — your own vault

The fastest path from zero to a working agent memory:

```bash
# 1. Build vaultmind from source (no binary releases yet)
git clone https://github.com/peiman/vaultmind && cd vaultmind

# 1a. (recommended) Install ORT acceleration for BGE-M3 indexing.
#     One-time. Requires libonnxruntime — `brew install onnxruntime` on macOS.
task setup:ort

# 1b. Build. Picks up ORT automatically when libtokenizers is present;
#     falls back to a pure-Go binary with a loud warning otherwise.
task build

# 2. Scaffold a fresh persona-shaped vault wherever makes sense for you
./vaultmind init "$HOME/.vaultmind/persona"

# 3. Index + embed
cd "$HOME/.vaultmind/persona"
/path/to/vaultmind index --vault .
/path/to/vaultmind index --embed --vault .

# 4. See what your agent would see
/path/to/vaultmind ask "who am I" --vault .
```

### About the build

`task build` produces an ORT-tagged binary when `lib/libtokenizers.a` is
present (installed by `task setup:ort`). ORT enables BGE-M3 sparse +
ColBERT lanes — the 4-way hybrid retrieval that VaultMind is built
around, and what the papers in `docs/` measure against.

Without ORT, the build falls back to pure-Go MiniLM-only embeddings
plus FTS — a working but degraded 2-lane retriever. The fallback is
loud (it tells you to run `task setup:ort`) and the binary still
works for the read path; only BGE-M3 indexing is hour-slow.

For fast inner-loop iteration on Go-only changes, `task build:fast`
skips ORT and produces a pure-Go binary in seconds.

`vaultmind init` produces a complete vault: type registry, vault README, starter notes for identity and current-context, and template arcs/principles to fill in. Edit `identity/who-am-i.md` and `references/current-context.md` from your agent's voice — those two files are the foundation everything else builds on.

To wire your vault into Claude Code (or any agent that supports SessionStart hooks), run `vaultmind hooks install <project-dir>` — the canonical hook scripts are embedded in the binary and get written into `<project-dir>/.claude/scripts/` (idempotent; refuses to clobber drift unless `--force`). The same scripts live at `.claude/scripts/` in this repo as a reference implementation.

**Or — let the agent set it up for you.** After `task build`, paste this sentence to Claude Code (substitute your actual clone path for `<vaultmind-clone>`):

> *I just installed vaultmind from `<vaultmind-clone>`. Walk me through onboarding — run `<vaultmind-clone>/vaultmind init --print-instructions` to read the agent-led setup script, then follow it.*

The clone path is required because `task build` produces `./vaultmind` inside the clone but does NOT install to PATH. Substituting the literal path lets the agent use it for every subsequent command in the onboarding flow without re-discovering it. The agent runs the command, reads the embedded onboarding script, asks you a few questions about who you are and what should be remembered, inspects your project, shows a diff-preview of every file it'll touch, and gets you to a wired vault. Greenfield (fresh persona) and migration (existing markdown) are both handled. See **[internal/onboard/AGENT_ONBOARDING.md](internal/onboard/AGENT_ONBOARDING.md)** for the full script if you want to read it directly.

## Try it with my vaults first

If you'd rather see VaultMind working before scaffolding your own, this repo ships with two vaults you can play with:

```bash
git clone https://github.com/peiman/vaultmind $HOME/dev/cli/vaultmind
cd $HOME/dev/cli/vaultmind
bash .claude/scripts/bootstrap.sh
```

The bootstrap builds the binary, embeds my `vaultmind-identity/` and `vaultmind-vault/`, and wires the SessionStart hook for this project. Idempotent — rerun to check / repair.

See **[SETUP.md](SETUP.md)** for the full guide, environment conventions, and troubleshooting.

## What it does

**Retrieval.** Hybrid search over a vault combining four lanes via Reciprocal Rank Fusion: full-text search, dense embeddings (BGE-M3 when ORT is set up, MiniLM fallback for pure-Go builds), BGE-M3 sparse embeddings, and ColBERT late-interaction. Notes are scored by per-lane RRF combined via mean-of-present.

**Calibrated confidence.** Every `vaultmind ask` result carries a `top_hit_confidence` label — `strong / moderate / weak / no_match` — derived from the rank-1/rank-2 score gap. The thresholds are calibrated against real probe queries (5%/1.5%/0.5%) so the label tells the agent whether to commit to top-1 or treat top-N as candidates.

**Persona reconstruction & in-session surfacing.** Four Claude Code hooks compose the integration: SessionStart loads identity + current-context, UserPromptSubmit injects per-turn pointers, PreToolUse(Read) tracks reads on vault files, and SessionEnd captures the session as an episode. The agent begins each session knowing who it is, sees relevant pointers as it works, and leaves a transcript behind for arc distillation.

**Episodic memory.** SessionEnd writes per-session transcripts (commits, files-touched, verbatim user/assistant messages) to `vaultmind-identity/episodes/`. This is the substrate for arc distillation — extracting transformation moments from real sessions rather than hand-authoring them.

**Activation tracking.** Every successful `ask` / `note get` / vault-Read fires `RecordNoteAccess` — so `notes.access_count` and `notes.last_accessed_at` reflect real use. ACT-R-shaped activation (`ln(count) − d · ln(t)`) is available as an opt-in retrieval rerank (slice 5b'').

**Frontmatter migration.** Per-vault `schema.aliases` lets users keep their existing field names (`last_updated`, `created_at`, etc.) without renaming on import. Aliases are non-destructive — vaultmind never rewrites your frontmatter.

**Observability.** Every retrieval event is logged with attribution: which agent (hook, CLI, specific consumer), which operator (user + host), which user-session (time-heuristic grouping of invocations), which query, which hits at which ranks with which score components. `vaultmind experiment summary` and `vaultmind experiment trace` read this data for weekly readouts and drill-down.

**Research platform.** The event log is structured for compressed-idle-time analysis (gamma parameter over session gaps), Hebbian edge strengthening (co-retrieval in user-sessions), and arc-recall distribution studies. See `docs/` for the research roadmap.

## Core concepts

- **Vault** — a directory of markdown notes with Obsidian-compatible frontmatter. Tracked in Git. Multiple vaults per project are supported (e.g. `vaultmind-identity/` for persona, `vaultmind-vault/` for research knowledge).
- **Arc** — a specific kind of note that describes a transformation rather than a fact. "Stepping back revealed X" not "principle: step back." Arcs are the atomic unit of persona, because identity is carried by the journey, not by the rules.
- **Activation** — ACT-R-inspired base-level activation: `ln(count) − d · ln(t_since_last)`. Tracked on every successful access (`ask`, `note get`, vault Read via the PreToolUse hook). Available as an opt-in retrieval rerank (slice 5b'') via `BuildAutoRetrieverWithRerank`; not the default ask path until calibrated-confidence thresholds are re-probed against the rerank's blended-score distribution.
- **Hybrid retrieval** — four lanes (FTS + dense + sparse + ColBERT) fused with RRF at K=60, mean-of-present normalization. The per-component scores are captured in the event log, so you can study what each lane contributes.
- **Top-hit confidence** — `top_hit_confidence` field on every `ask` result: `strong` (gap ≥5%) / `moderate` (≥1.5%) / `weak` (≥0.5%) / `no_match` (below). Lets the agent decide when to commit to top-1 vs treat top-N as candidates. Thresholds calibrated against probe queries; pinned by regression test.
- **Session attribution** — every event carries `caller` (agent label), `caller_meta.user` / `caller_meta.host` (operator), and `user_session_id` (grouping of invocations within 30 minutes of the same caller+user+host).
- **Frontmatter aliases** — `.vaultmind/config.yaml` accepts `schema.aliases` (canonical → list of alternative names). The validator and mutation surface treat canonical and alias as equivalent; vaults migrating from other systems keep their existing field names without renames.
- **Vault fingerprint** — anonymous per-vault grouping ID generated at `vaultmind init`; the basis for federated rollup (Paper #2 substrate).

## Key commands

```bash
# Scaffold + retrieval
vaultmind init <path>                                        # scaffold a fresh persona-shaped vault
vaultmind init --print-instructions                          # print the embedded agent-onboarding script (no vault created)
vaultmind ask "who am I" --vault <path>                      # menu + context-pack (default)
vaultmind ask "who am I" --vault <path> --pointers-only      # menu only — cheapest, no bodies
vaultmind note get <id> --vault <path>                       # read one note by id (fires access tracking)
vaultmind search "judgment" --vault <path> --mode hybrid

# Inspect the agent's own memory state
vaultmind self --vault <path>                                # recent / hot / stale notes

# Vault maintenance
vaultmind index --vault <path>                               # (re)build the index
vaultmind index --embed --vault <path>                       # compute embeddings (auto-default: bge-m3 on ORT, minilm on pure-Go)
vaultmind index --embed --model minilm --vault <path>        # force minilm even on ORT (fast iteration during dev)
vaultmind doctor --vault <path>                              # health check: schema, embeddings, links

# Frontmatter mutations (alias-aware)
vaultmind frontmatter set <id> <key>=<value> --vault <path>
vaultmind frontmatter unset <id> <key> --vault <path>
vaultmind frontmatter merge --file <path-to-note> --fields k1=v1,k2=v2

# Observability
vaultmind experiment summary                                 # top recalled notes, session gap stats
vaultmind experiment trace --session <id>                    # what one session retrieved
vaultmind experiment trace --note <id>                       # which sessions retrieved this note

# Telemetry export (early adopters who opted into anonymous/full sharing)
vaultmind export --rollup                                    # federated payload shape (per-vault metrics + variant rollup)
vaultmind export --tier anonymous                            # JSONL snapshot for sharing
vaultmind export --output ./vm-export.jsonl                  # write to file instead of stdout
```

Run `vaultmind --help` for the full list.

**For AI agents using VaultMind as memory:** see **[docs/AGENT_USAGE.md](docs/AGENT_USAGE.md)** — end-to-end guide covering save, retrieve, update, inspect, frontmatter schema, and integration patterns.

**For AI agents helping a new user set vaultmind up:** run `vaultmind init --print-instructions` (the doc is embedded in the binary, so it works wherever vaultmind is installed). Source: **[internal/onboard/AGENT_ONBOARDING.md](internal/onboard/AGENT_ONBOARDING.md)** — one-time setup walkthrough covering preflight, project read, greenfield + migration paths, hook wiring with diff-preview, verification, and failure modes.

## How the hooks work

VaultMind integrates with Claude Code via four hooks at different lifecycle points. All four are wired in `.claude/settings.json` for this project; reference implementations live in `.claude/scripts/`. Adapt the same shape for your own project.

**SessionStart — `load-persona.sh`**
1. Rebuilds `/tmp/vaultmind` when any `.go` source is newer than the binary (auto-propagates your VaultMind commits to the next session).
2. Runs `vaultmind ask "who am I"` (full bodies — priming the identity layer) and `vaultmind ask "what matters most right now" --pointers-only` (forces query-then-read for the live edge).
3. Runs `vaultmind self` for both vaults — the agent sees its own recent / hot / stale activation state.
4. Captures stderr; surfaces build / runtime errors instead of silently producing empty persona.
5. Emits an `IDENTITY CONTEXT` block as a `system-reminder` in the agent's session.

**UserPromptSubmit — `vault-recall.sh`**
1. Runs `vaultmind ask "<user-prompt>" --pointers-only --max-items 3` against the identity vault on every user message above ~12 chars.
2. Surfaces the top-3 pointers as a `system-reminder` before the agent responds — associative recall at conversation-turn granularity.
3. Skipped silently for short single-word prompts ("yes", "ok") and substrate-not-ready states.

**PreToolUse(Read) — `vault-track-read.sh`**
1. Detects when Claude reads a markdown file under any `vaultmind-*/` directory.
2. Fires `vaultmind note get` for access tracking AND injects a `system-reminder` naming the canonical retrieval command.
3. Read still proceeds — the hook does not block. (A blocking variant `vault-block-read.sh` is parked on disk for future escalation.)

**SessionEnd — `capture-episode.sh`**
1. Captures the full session transcript (commits, PRs, files-touched, verbatim user/assistant messages) to `vaultmind-identity/episodes/episode-<date>-<id>.md`.
2. The substrate for arc distillation — transformation moments are extracted from real episodes rather than hand-authored.

The four hooks compose: identity primes at start; pointers surface mid-session; reads are tracked; episodes are captured at session end. Together they make a vault behave as the agent's working memory across sessions.

## Status

Pre-v0.1.0. Actively dogfooded. No release binaries cut yet — the distribution pipeline (GoReleaser, GitHub Actions, optional Homebrew tap) is set up but waits on dogfood validation before tagging. Today, install means clone + `task build`.

The work is organized around the **plasticity roadmap** in `vaultmind-identity/references/plasticity-priority-order.md` — six steps each a platform for the next:

1. **Episodic substrate** — per-session transcripts captured automatically. ✅ shipped.
2. **Arc distillation** — extracting transformation moments from episodes. (Substrate ready; tool not yet built.)
3. **Activation-triggered recall** — pointers surfaced per turn. ✅ slices landed.
4. **Calibrated confidence** — `top_hit_confidence` labels with probed thresholds. ✅ first slice.
5. **Decay + reinforcement** — base-level activation wired into retrieval. Tracking ✅; rerank shipped opt-in (`BuildAutoRetrieverWithRerank`); default-on gated on a calibration re-probe of the rerank's score-gap distribution.
5.5. **Federation read** — cross-vault retrieval as a lighter alternative to mega-vault. Designed in `vaultmind-identity/references/federation-architecture.md`; not yet implemented.
6. **MCP / cross-agent memory** — write-capable cross-agent collaboration. Deferred until step 5 closes.

Telemetry export (`vaultmind export`) ships sanitized JSONL snapshots that early adopters can share back; the federated rollup payload shape is the empirical substrate for Paper #2. The upload pipeline is intentionally manual until enough early-adopter signal arrives to design the receiver.

## Contributing

This is a research-grade project that's also a working tool — quality standards are high by design. Before contributing:

- Read `AGENTS.md` and `CLAUDE.md` for the project's architecture rules and workflow
- Use `task check` as the single quality gate (runs framework checks + VaultMind bootstrap verification + hook smoke test)
- Follow TDD: write failing tests first, commit test + implementation together, atomic commits
- Read `SETUP.md` for the bootstrap path; run `bash .claude/scripts/bootstrap.sh --check` to verify your environment

## Research & license

Built in partnership with Peiman Khorramshahi. Research artifacts in `docs/` include the evaluation protocol, review rounds, and planning documents for the compressed-idle-time paper. Data quality protections (caller attribution, user-session grouping, content-drift detection on embeddings) are built-in and enforced at the schema level.

Built on [ckeletin-go](https://github.com/peiman/ckeletin-go) — the underlying scaffold providing ultra-thin commands, centralized configuration, automated architecture validation, and updatable framework layer. The `.ckeletin/` directory can be updated independently via `task ckeletin:update`.

MIT License. See [LICENSE](LICENSE).
