# Changelog

All notable changes to VaultMind are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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

[Unreleased]: https://github.com/peiman/vaultmind/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/peiman/vaultmind/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/peiman/vaultmind/compare/v0.1.3...v0.1.4
[0.1.3]: https://github.com/peiman/vaultmind/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/peiman/vaultmind/releases/tag/v0.1.2
