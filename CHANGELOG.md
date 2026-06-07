# Changelog

All notable changes to VaultMind are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.11] — 2026-06-07

### Added
- **`doctor heal` — repair lives under the health hub.** `vaultmind doctor heal` applies every
  auto-fixable repair `doctor` found (today: Obsidian-incompatible wikilinks); `doctor heal wikilinks`
  is the surgical form (the logic moved here from the removed `lint fix-links`). `heal` **applies by
  default**; `--dry-run` previews. Cobra alias `fix` works on both (`doctor fix`, `doctor fix wikilinks`).
  All `doctor heal *` paths share one mutation engine (`internal/mutation`).
- **`doctor --summary` — the cold-start view.** Counts, the per-type breakdown that `vault status`
  produced, and an errors/warnings rollup, in one read-only command.
- **`doctor --all` — health for every vault at once.** Walks `--root` (default `.`, bounded depth)
  for directories containing a `.vaultmind/` and runs the diagnosis on each, printing a combined
  rollup plus per-vault sections; `--json` emits one combined envelope; vaults that fail to open are
  surfaced (named with their error), not silently skipped. Explicit opt-in — bare `doctor` and
  `doctor --vault` are unchanged.
- **`vaultmind help` now lists every command, grouped by intent, each with a when-to-use line.** The
  catalog is generated from a single source (the cobra command tree) and also backs the new
  `vaultmind docs commands` (→ `COMMANDS.md`) and the agent onboarding (`init --print-instructions
  --full`). An enforcement test keeps every command catalogued; a drift test keeps the embedded
  `COMMANDS.md` in sync.

### Changed
- **Graph traversal is unified under `memory`.** `memory links <id> [--out|--in|--both]` (default
  `--both`; `--in` = backlinks) is a single direction-flagged command that absorbs the old
  `links out` / `links in`. `memory neighbors <id> [--depth N]` is the BFS neighborhood with full
  frontmatter (merging the old `links neighbors` and `memory recall`). `memory context-pack` is
  renamed to `memory pack` (identical behavior). `memory related` / `memory summarize` unchanged.
- **`doctor` is now the single vault-health hub.** Read-only `doctor` gained the per-type breakdown
  that `vault status` carried, and repair lives under it via `doctor heal`. The diagnosis remedy for
  broken links now points at `vaultmind doctor heal wikilinks` (previously an unshipped helper script).

### Deprecated
- The following invocations are now **hidden deprecated aliases** that print a one-line stderr notice
  and delegate to the new path. They will be removed in ~2 releases:
  - `links out` → `memory links --out`
  - `links in` → `memory links --in`
  - `links neighbors` → `memory neighbors`
  - `lint` (top-level, removed) and `lint fix-links` → `doctor heal wikilinks`
  - `vault status` → `doctor --summary` (the `vault` parent is deprecated; it only hosted `status`).
    Note: `vault status --json` now returns the **doctor envelope shape** (the `doctor` result —
    different from the old `StatusResult`), since the alias delegates to doctor's JSON path. Consumers
    that decoded the old `vault status --json` payload must update to the doctor result shape.
  - `memory recall` → `memory neighbors`
  - `memory context-pack` → `memory pack`
- The canonical repair verb is `heal`; `fix` is a permanent cobra alias (help shows "heal (fix)").
  `dataview lint` is a separate domain checker and is **not** affected.

## [0.1.10] — 2026-06-05

### Added
- **Concise quick-start for agent onboarding.** `vaultmind init --print-instructions` now prints a
  short, skimmable quick-start (install → `init` → `hooks install --vault` → the env-var routing
  table → `index --embed` → first `ask`) instead of the full 700-line guide an agent had to read in
  chunks. The complete guide is still one flag away: `vaultmind init --print-instructions --full`.
- **Per-concern vault routing for the hooks.** A single overloaded `VAULTMIND_VAULT` used to drive
  persona-load, per-turn recall, AND episode-writes — so a two-vault adopter (a personal identity
  vault + a shared knowledge vault) couldn't route them independently. New `VAULTMIND_RECALL_VAULT`
  (per-turn recall + read-tracking) and `VAULTMIND_EPISODE_VAULT` (episode writes) each **fall back to
  `VAULTMIND_VAULT`**, so existing single-var setups are unchanged.

### Changed
- **`vaultmind init --print-instructions` now defaults to the quick-start, not the full guide.** Use
  `--full` for the previous whole-guide output. (Behavior change for anyone scripting around the old
  full dump.)
- Onboarding docs clarified: the `LOAD_PERSONA_RESEARCH_VAULT` block runs only `vaultmind self` (a
  memory/activation-state surface — hot/recent note titles), not a content `ask`; and `index --embed`
  is content-hash incremental (only new/changed notes embed), so per-note live indexing is cheap.

## [0.1.9] — 2026-06-04

### Fixed
- **Hook-drift detection no longer false-positives on comment-only differences.**
  `doctor`'s hook-drift check compared each installed hook script to the embedded
  canonical byte-for-byte, so a script that kept richer annotations than the shipped
  (sanitized) canonical was reported as "drifted" even when its code was identical —
  training you to ignore a diagnostic that was crying wolf. It now compares the
  behavioral skeleton (full-line comments and blank lines stripped; heredoc bodies and
  quoted-string contents preserved), so only a real **code** change counts as drift.
  This matches the "only real edits are drift" doctrine already used for vault-note
  drift. Backed by a new heredoc- and quoting-aware `shellparse.StripCommentsAndBlanks`.

## [0.1.8] — 2026-06-04

### Fixed
- **`episodes/` is now excluded from indexing by default.** Captured session
  transcripts (the bootstrap target) are raw material for arc distillation, not
  retrieval targets — large (a transcript is ~30× the size of an arc) and redundant
  with the arcs distilled from them. The `init` template and `defaultExcludes` now
  exclude `episodes`, so a bootstrapped vault doesn't embed megabytes of transcripts.
  (Existing vaults: add `- "episodes"` to your `.vaultmind/config.yaml` exclude list.)

## [0.1.7] — 2026-06-04

Re-release of 0.1.6 with prebuilt binaries. 0.1.6's release job failed the coverage
gate before building artifacts, so 0.1.6 is `go install`-able but ships no prebuilt
ORT archives; 0.1.7 supersedes it (0.1.6 is retracted in `go.mod`). Same features.

### Fixed
- Coverage floor: the `episode capture` command (single-file and directory paths)
  had no cmd-level test, which dropped project coverage below the gate. Added one.

### Changed
- README now surfaces the cold-start **bootstrap-from-transcripts** path and the
  example vault's concept cards, and notes the "Try it" commands assume a repo
  checkout (clarifying it for `go install` / prebuilt-archive users).

## [0.1.6] — 2026-06-04

### Added
- **Bootstrap an identity vault from existing transcripts.** `vaultmind episode
  capture` now accepts a **directory** — it recursively captures every `*.jsonl`
  transcript under it into episodes (empty/non-transcript files skipped), so you can
  seed a vault from months of existing Claude Code sessions in one command, then run
  `vaultmind arc candidates`. The agent-onboarding guide gains a step that offers this
  during setup; the identity guide gains a "cold start" section. (`capture` now also
  gates on a real session id, so junk transcripts no longer produce degenerate episodes.)
- **Concept cards in the example vault** (`examples/ada-vault/concepts/`) — atomic
  notes defining the core vocabulary an adopter needs: **arc**, **episode**,
  **principle**, and **the-memory-pipeline** (how they link: episode → arc candidate
  → arc → principle; arcs anchor identity). The example vault now teaches the model it
  demonstrates. Complements [docs/building-an-identity-vault.md](docs/building-an-identity-vault.md).

### Fixed
- **`vault-track-read.sh` aborted with "unbound variable" under `set -u`.** The
  PreToolUse read-tracking hook referenced the *optional* `$VAULT_PATH_PATTERN` /
  `$VAULTMIND_VAULT` env vars bare; under `set -u` (which the script sets) an unset
  optional var aborts the hook on every vault Read (non-blocking, but noisy and the
  tracking silently didn't run). Guarded both with `${VAR:-}` defaults; added a
  regression test pinning it (field report 2026-06-04).

## [0.1.5] — 2026-06-04

### Added
- **New guide: [docs/building-an-identity-vault.md](docs/building-an-identity-vault.md).**
  How to *grow* an agent's identity vault — the arc method (identity is carried by
  transformation moments, not rules; you don't author it up front, it accretes from
  real sessions) — and a prominent boundary: **an identity vault is personal and
  should not be committed to a shared project repo** unless you deliberately want one
  shared identity across all developers. Linked from the README, the agent-onboarding
  guide (§4a), and the example vault; the onboarding now tells the agent to surface
  the personal-vs-shared choice during setup.

### Changed
- **`index --embed` now names the MiniLM lane gap at embed time.** A pure-Go
  (`go install`) build silently lands on MiniLM (dense-only, 2 lanes). The embed
  output now adds a one-line note — dense-only + the **no-compile** upgrade to the
  full BGE-M3 hybrid via the prebuilt ORT archive — so an adopter learns it at the
  moment of indexing, not only from a later `doctor` run (focalc/Patrik field report).
- **README install section clarifies the MiniLM vs BGE-M3 choice.** A "Which one?"
  callout: MiniLM is genuinely fine for small vaults / slow machines; BGE-M3 (the
  prebuilt ORT archive, no compiler) is for large/varied vaults wanting best recall;
  and `go install` is the only path that can't produce BGE-M3 — so a `go install`-based
  setup is on MiniLM by design.

## [0.1.4] — 2026-06-04

### Fixed
- **`vaultmind version` on `go install` builds** — a `go install …@version` binary printed `version dev, commit , built at ` (empty commit/date) because ldflags aren't injected on that path, even though Go embeds the module version and VCS stamps. Both `version` and `--version` now fall back to `debug.ReadBuildInfo()` (module version + VCS revision/time). Release binaries built with ldflags are unchanged.
- **Empty `vaultmind search` output on zero hits** — a text-mode search with no matches printed nothing and exited 0, indistinguishable from a broken command. It now names the empty result and points at `vaultmind ask` for paraphrase matching.
- **Embed remedy hints no longer suggest a refused command** — the "no embeddings yet" hints (`doctor`'s none-state, keyword-only `ask`) recommended `index --embed --model bge-m3`, which the pure-Go binary `go install` yields *refuses*. They now suggest plain `vaultmind index --embed`, letting the backend pick its default model (bge-m3 on ORT, minilm on pure-Go). The bge-m3-specific modality-imbalance hint is ORT-only and unchanged.
- **A vault's own `README.md` no longer pollutes retrieval** — vault scanning now excludes files by basename or vault-relative path (it previously filtered directories only), and `README.md` is excluded by default and in the `init` config template. The vault's meta README is no longer indexed as a blank-titled note surfacing in every query's results.

## [0.1.3] — 2026-06-04

First installable public release of the VaultMind CLI — a single-binary
associative-memory engine for AI agents over Git-backed Markdown vaults.
Supersedes the retracted 0.1.0–0.1.2 versions (see Removed); `go install
github.com/peiman/vaultmind@latest` resolves here.

### Added
- Vault indexing: full-text (FTS5) + BGE-M3 dense/sparse/ColBERT embeddings + a
  link/alias knowledge graph, built with `vaultmind index`.
- 4-way Reciprocal Rank Fusion hybrid retrieval with calibrated top-hit
  confidence and optional activation-weighted reranking.
- `vaultmind ask` — token-budgeted context packs; stable `--json` envelope on
  every command.
- `vaultmind init` — scaffolds a fresh vault (type registry, README, and starter
  identity / principle / arc notes), with optional one-command Claude Code wiring
  via `--wire-hooks` and an agent-led setup walkthrough via `--print-instructions`.
- Persona-reconstruction hooks for Claude Code via `vaultmind hooks install`, with
  their reference scripts shipped under `.claude/scripts/`.
- Pure-Go MiniLM build (`go install`) and prebuilt self-contained ONNX Runtime
  archives (BGE-M3) for `darwin-arm64` and `linux-amd64`.
- A fictional example vault at `examples/ada-vault`.
- Opt-in, sanitized usage telemetry (counts and identifiers only).

### Removed
- Retracted v0.1.0–v0.1.2 in `go.mod`: withdrawn versions on this module path that
  predate this release and are superseded by it.

## [0.1.2] — withdrawn

The initial public tag, retracted in favor of [0.1.3]. It shipped without the
`.claude/scripts/` reference hook scripts its own onboarding references, and carried
maintainer-only CI steps — both corrected in 0.1.3. Kept here for the record; do
not install.

[Unreleased]: https://github.com/peiman/vaultmind/compare/v0.1.11...HEAD
[0.1.11]: https://github.com/peiman/vaultmind/compare/v0.1.10...v0.1.11
[0.1.10]: https://github.com/peiman/vaultmind/compare/v0.1.9...v0.1.10
[0.1.9]: https://github.com/peiman/vaultmind/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/peiman/vaultmind/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/peiman/vaultmind/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/peiman/vaultmind/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/peiman/vaultmind/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/peiman/vaultmind/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/peiman/vaultmind/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/peiman/vaultmind/releases/tag/v0.1.2
