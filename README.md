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

To wire your vault into Claude Code (or any agent that supports SessionStart hooks), see `.claude/scripts/load-persona.sh` in this repo as a reference implementation.

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

**Retrieval.** Hybrid search over a vault combining four lanes via Reciprocal Rank Fusion: full-text search, dense embeddings (MiniLM by default, BGE-M3 optional), sparse embeddings, and ColBERT late-interaction. Scored by ACT-R-inspired activation with spreading activation via cosine similarity.

**Persona reconstruction.** A SessionStart hook runs `vaultmind ask "who am I"` against a persona vault at the start of every agent session. The hook injects the top-activated identity notes (arcs, principles, current context) as a system-reminder — so the agent begins the session knowing who it is and what matters.

**Observability.** Every retrieval event is logged with attribution: which agent (hook, CLI, specific consumer), which operator (user + host), which user-session (time-heuristic grouping of invocations), which query, which hits at which ranks with which score components. `vaultmind experiment summary` and `vaultmind experiment trace` read this data for weekly readouts and drill-down.

**Research platform.** The event log is structured for compressed-idle-time analysis (gamma parameter over session gaps), Hebbian edge strengthening (co-retrieval in user-sessions), and arc-recall distribution studies. See `docs/` for the research roadmap.

## Core concepts

- **Vault** — a directory of markdown notes with Obsidian-compatible frontmatter. Tracked in Git. Multiple vaults per project are supported (e.g. `vaultmind-identity/` for persona, `vaultmind-vault/` for research knowledge).
- **Arc** — a specific kind of note that describes a transformation rather than a fact. "Stepping back revealed X" not "principle: step back." Arcs are the atomic unit of persona, because identity is carried by the journey, not by the rules.
- **Activation** — ACT-R-inspired scoring: base-level activation (frequency + recency) plus spreading activation when query similarity is available. Notes you've accessed recently and notes semantically close to your query rise to the top.
- **Hybrid retrieval** — four lanes (FTS + dense + sparse + ColBERT) fused with RRF at K=60. The per-component scores are captured in the event log, so you can study what each lane contributes.
- **Session attribution** — every event carries `caller` (agent label), `caller_meta.user` / `caller_meta.host` (operator), and `user_session_id` (grouping of invocations within 30 minutes of the same caller+user+host).

## Key commands

```bash
# Scaffold + retrieval
vaultmind init <path>                                        # scaffold a fresh persona-shaped vault
vaultmind ask "who am I" --vault <path> --max-items 8 --budget 6000
vaultmind search "judgment" --vault <path> --mode hybrid

# Vault maintenance
vaultmind index --vault <path>                               # (re)build the index
vaultmind index --embed --model minilm --vault <path>        # compute embeddings
vaultmind doctor --vault <path>                              # health check: notes, embeddings, links

# Observability
vaultmind experiment summary                                 # top recalled notes, session gap stats
vaultmind experiment trace --session <id>                    # what one session retrieved
vaultmind experiment trace --note <id>                       # which sessions retrieved this note

# Telemetry export (early adopters who opted into anonymous/full sharing)
vaultmind export --tier anonymous                            # JSONL snapshot for sharing
vaultmind export --output ./vm-export.jsonl                  # write to file instead of stdout
```

Run `vaultmind --help` for the full list.

**For AI agents using VaultMind as memory:** see **[docs/AGENT_USAGE.md](docs/AGENT_USAGE.md)** — end-to-end guide covering save, retrieve, update, inspect, frontmatter schema, and integration patterns.

## How the hook works

VaultMind integrates with Claude Code (and other hook-supporting agents) via a SessionStart hook script. Two reference implementations are in this repo:

- `.claude/scripts/load-persona.sh` — loads **this project's** persona from `vaultmind-identity/` at session start
- Workhorse repo's `.claude/scripts/load-persona.sh` — same pattern, different vault, different caller label

The hook:
1. Rebuilds `/tmp/vaultmind` when any `.go` source is newer than the binary (auto-propagates your VaultMind commits to the next session)
2. Runs `vaultmind ask "who am I"` with `VAULTMIND_CALLER=<project>-persona-hook` for clean attribution
3. Captures stderr and surfaces build / runtime errors instead of silently producing empty persona
4. Emits an `IDENTITY CONTEXT` block to stdout that becomes a system-reminder in the agent's session

## Status

Pre-v0.1.0. Actively dogfooded. No release binaries cut yet — the distribution pipeline (GoReleaser, GitHub Actions, optional Homebrew tap) is set up but waits on dogfood validation before tagging. Today, install means clone + `task build`.

The research foundation (retrieval, attribution, observability) is built and generating data from real sessions. The paper on compressed idle time is in progress. Telemetry export (`vaultmind export`) ships sanitized JSONL snapshots that early adopters can share back; the upload pipeline is intentionally manual until enough early-adopter signal arrives to design the receiver.

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
