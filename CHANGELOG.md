# Changelog

All notable changes to VaultMind are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.2] — first public release

First public release of the VaultMind CLI — a single-binary associative-memory
engine for AI agents over Git-backed Markdown vaults.

### Added
- Vault indexing: full-text (FTS5) + BGE-M3 dense/sparse/ColBERT embeddings + a
  link/alias knowledge graph, built with `vaultmind index`.
- 4-way Reciprocal Rank Fusion hybrid retrieval with calibrated top-hit
  confidence and optional activation-weighted reranking.
- `vaultmind ask` — token-budgeted context packs; stable `--json` envelope on
  every command.
- `vaultmind init` — scaffolds a fresh vault with a type registry and starter notes.
- Persona-reconstruction hooks for Claude Code via `vaultmind hooks install`.
- Pure-Go MiniLM build (`go install`) and prebuilt self-contained ONNX Runtime
  archives (BGE-M3) for `darwin-arm64` and `linux-amd64`.
- A fictional example vault at `examples/ada-vault`.
- Opt-in, sanitized usage telemetry (counts and identifiers only).

[Unreleased]: https://github.com/peiman/vaultmind/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/peiman/vaultmind/releases/tag/v0.1.2
