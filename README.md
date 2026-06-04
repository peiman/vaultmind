# VaultMind

**Associative memory for AI agents — over Git-backed Markdown vaults.**

AI agents start each session from zero: strong parametric knowledge, but no memory of what they did yesterday, what they decided, or what they're working toward. VaultMind turns a directory of Markdown notes into queryable, activation-weighted memory an agent can reconstruct itself from at the start of a session — and *continue* from, rather than start over.

It's a single Go binary. Point it at a vault of Markdown files with Obsidian-compatible frontmatter; it builds a derived index (full-text + dense + sparse + late-interaction embeddings), resolves `[[wikilinks]]` and aliases into a knowledge graph, and answers queries with ranked, token-budgeted context.

## Install

**Quick — MiniLM, every platform:**

```bash
go install github.com/peiman/vaultmind@latest   # requires Go >= 1.26.4
```

A pure-Go binary: full-text + MiniLM dense retrieval (2 lanes). Ideal for trying VaultMind — it does **not** include BGE-M3's sparse + ColBERT lanes.

**Full 4-way hybrid — BGE-M3 (recommended for real use):**

Download the self-contained ORT archive for your platform (`darwin-arm64`, `linux-amd64`) from the [releases page](https://github.com/peiman/vaultmind/releases) — `vaultmind_<version>_<os>_<arch>_ort.tar.gz` — extract, and run. The official `libonnxruntime` is bundled (found automatically next to the binary), so there's nothing else to install — **no compiler required.**

> **Which one?** For a **small vault or a slow machine, MiniLM is genuinely fine** — its dense lane covers a small corpus well, and its lighter query encoder keeps per-prompt latency low (that cost lands on *every* recall). Reach for **BGE-M3 when your vault is large and varied** and recall quality matters most. Note: `go install` is the *only* path that can't produce BGE-M3 (cgo can't travel through it) — so if your setup uses `go install`, you're on MiniLM by design; the prebuilt archive above is the no-compile way to the full hybrid.

**From source (any platform with a C toolchain):**

```bash
git clone https://github.com/peiman/vaultmind && cd vaultmind
brew install onnxruntime   # macOS; Linux: install from the ONNX Runtime releases
task setup:ort             # downloads the tokenizer static lib
task build                 # auto-selects ORT when the tokenizer lib is present
```

See **[docs/embedding-backends.md](docs/embedding-backends.md)** for every backend, platform, performance note, and the dense-only vs. 4-way-hybrid tradeoff.

## Quickstart

```bash
# 1. scaffold a vault (type registry, README, starter notes)
vaultmind init ./my-vault

# 2. index + embed
vaultmind index --vault ./my-vault
vaultmind index --embed --vault ./my-vault

# 3. ask
vaultmind ask "what did we decide about retries?" --vault ./my-vault
```

## Try it with the example vault

VaultMind ships a small **fictional** example vault — *Ada*, an agent that pair-programs with a developer named Sam on a toy CLI — so you can see retrieval and persona reconstruction working before you build your own:

```bash
vaultmind index --vault examples/ada-vault
vaultmind index --embed --vault examples/ada-vault     # BGE-M3 on an ORT build, MiniLM otherwise
vaultmind ask "who are you" --vault examples/ada-vault
vaultmind ask "what did Ada learn about scope?" --vault examples/ada-vault
```

## How it works

- **Vault** — a directory of Markdown notes with Obsidian-compatible frontmatter, tracked in Git. You curate it; the agent reads it.
- **Index** — a derived SQLite index: full-text (FTS5), dense + sparse + ColBERT embeddings (BGE-M3), and a link/alias knowledge graph. Rebuilt with `vaultmind index`; never hand-edited.
- **Retrieval** — Reciprocal Rank Fusion over the lanes, with calibrated top-hit confidence and optional **activation-weighted reranking** (notes accessed more often become more retrievable).
- **Context packs** — `vaultmind ask` assembles ranked results into a token-budgeted block ready to drop into an agent's context.

Everything is `--json`-able for programmatic use; every command returns a stable envelope.

## Agent integration (persona reconstruction)

VaultMind can wire into Claude Code — or any agent that supports SessionStart hooks — so the agent reconstructs itself from its vault each session:

```bash
vaultmind hooks install <project-dir>
```

This installs hook scripts that load identity + current context at session start, surface relevant pointers per turn, and capture each session as an episode for later distillation. The scripts are embedded in the binary and written into `<project-dir>/.claude/scripts/` (idempotent). See **[docs/AGENT_USAGE.md](docs/AGENT_USAGE.md)** for the day-to-day agent workflow.

## Opt-in usage telemetry

VaultMind records local retrieval events (which queries surfaced which notes) to power activation-weighted reranking. Sharing that data is **opt-in and sanitized** — no note bodies, no content, no query text; only counts and identifiers — for anyone who wants to contribute anonymized retrieval signal back to the project. It is off by default and never leaves your machine unless you run the export.

## Contributing

VaultMind is a Go CLI built on the [ckeletin-go](https://github.com/peiman/ckeletin-go) scaffold (the `.ckeletin/` framework layer). `task check` is the single quality gate — it runs formatting, linting, architecture and security checks, the full test suite, and the coverage floor. If it passes, the change is sound regardless of who wrote it.

- Read **[AGENTS.md](AGENTS.md)** for architecture rules and conventions, and **[CONTRIBUTING.md](CONTRIBUTING.md)** before opening a PR.
- TDD: write the failing test first; commit test + implementation together.

## License

See **[LICENSE](LICENSE)**.
