# VaultMind

**Associative memory for AI agents — persona reconstruction and semantic retrieval over Git-backed Obsidian vaults.**

---

## The problem

AI agents start each session from zero. They have strong parametric knowledge but no autobiographical memory, no arcs of growth, no continuity of identity across conversations. The mind that traced 140,000 lines of code yesterday is the same mind facing a blank prompt today.

VaultMind closes that gap. A vault of markdown notes becomes queryable, activation-weighted memory that an agent can reconstruct itself from at session start — and continue from, not start from.

## TL;DR

```bash
git clone https://github.com/peiman/vaultmind $HOME/dev/cli/vaultmind
cd $HOME/dev/cli/vaultmind
bash .claude/scripts/bootstrap.sh
```

That's it. The bootstrap script builds the binary, embeds your vaults, and verifies the SessionStart hook is wired. Idempotent — rerun any time to check / repair.

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
# Retrieval
vaultmind ask "who am I" --vault vaultmind-identity --max-items 8 --budget 6000
vaultmind search "judgment" --vault vaultmind-vault --mode hybrid

# Vault maintenance
vaultmind index --vault <path>                               # (re)build the index
vaultmind index --embed --model minilm --vault <path>        # compute embeddings
vaultmind doctor --vault <path>                              # health check: notes, embeddings, links

# Observability
vaultmind experiment summary                                 # top recalled notes, session gap stats
vaultmind experiment trace --session <id>                    # what one session retrieved
vaultmind experiment trace --note <id>                       # which sessions retrieved this note
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

Pre-v0.1.0. Actively dogfooded. No release binaries cut yet — the distribution pipeline (GoReleaser, GitHub Actions, optional Homebrew tap) is set up but waits on dogfood validation before tagging.

The research foundation (retrieval, attribution, observability) is built and generating data from real sessions. The paper on compressed idle time is in progress.

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
