# Changelog

All notable changes to VaultMind are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`arc candidates` shows the existing arcs each proposal resembles, with cosine scores.** The
  2026-05-31 distillation review named de-duplication — not extraction — the biggest risk in this
  pipeline, having measured two independent miners mis-tagging the same candidate to two different
  existing arcs, both wrong. Every proposal now carries its top-3 nearest arcs. The tool finds the
  neighbours and deliberately refuses the covered/new verdict: a duplicate proposal costs seconds
  of reading, while a wrong "already covered" silently discards a transformation nobody will look
  for again. New `--arcs-vault` points the comparison at a different vault, for setups where raw
  entries and curated arcs live apart.
- **`arc candidates` now reads the desk, not just session episodes.** The scanner grepped
  transcripts for a handful of phrases while the notes a mind writes *specifically* to record its
  own transformations sat unread one directory over. Journal-type notes in the scanned vault are
  now surfaced as pending arc material, above the phrase-matched moments — a candidate is a guess
  ("a phrase fired, go look"), a desk entry is a judgement already made, and printing the guesses
  first buried the judgements. Mark one done with `distilled_to: <arc-id>` in its frontmatter; the
  state lives on the note rather than in a cross-vault link. Pointing at an identity vault reads
  its episodes, pointing at a desk vault reads its entries — no new flag.

### Fixed
- **`arc candidates` fails closed on a vault path that does not exist.** It scans directories
  directly rather than opening the vault DB, so nothing validated `--vault`: a typo produced
  `Scanned 0 episodes → 0 candidate moments` and exit 0, indistinguishable from a real vault with
  nothing pending — two states calling for opposite responses (fix the path vs. go write
  something).
- **The empty-report line no longer contradicts the entries above it.** With desk entries pending
  but no phrase matches, the report printed "No candidate moments found" directly beneath the list
  it had just rendered.

## [0.3.0] — 2026-08-13

### Added
- **`episode capture --incremental` captures a session's transcript in bounded segments instead
  of re-rendering the whole thing on every call.** A long-lived session that never closes used to
  have its `SessionEnd` hook re-render the ENTIRE transcript into the same episode file every
  invocation, growing that file without bound. With `--incremental`, each capture resumes from a
  cursor (persisted per session and target vault under `--cursor-dir`, defaulting to the XDG state
  dir) and writes only the new content as its own small `-partNNNNNNNN` segment, so an
  ever-growing session produces several bounded files instead of one unbounded one. (#72)

### Fixed
- **Vault auto-discovery no longer silently selects your home directory.** When no `--vault` was
  given, the walk-up looked for a `.vaultmind/` from the working directory all the way to the
  filesystem root. A stray `.vaultmind/` at `$HOME` — the debris one errant `vaultmind index` run
  from the home directory leaves behind — therefore captured *every* invocation made anywhere
  beneath `$HOME` that wasn't already inside another vault. `ask` answered from that wrong vault
  while reporting `status: "ok"`, and the remedy it printed for the resulting zero hits
  (`vaultmind index --embed`) aimed the indexer at the entire home directory. `$HOME` is now never
  auto-selected; the walk continues past it, so a real vault below (or deliberately above) it still
  resolves, and naming it explicitly via `--vault` or `VAULTMIND_VAULT` still works. This blocks
  the accident, not the choice.
- **A command that had to *guess* its vault now fails closed instead of answering from a
  non-vault directory.** `vault.LoadConfig` deliberately treats any directory as a usable vault, so
  when discovery fell back to `"."` a command would happily query whatever directory you were
  standing in and report `status: "ok"` with zero hits — which reads as "your vault has nothing on
  this topic" when the truth is "you have no vault", two answers that call for opposite next steps.
  Opening it also *created* `.vaultmind/index.db` there, promoting that directory to a vault every
  future walk-up would find, so the mistake propagated itself. A guessed path must now already be a
  vault; a named one (`--vault`, `VAULTMIND_VAULT`) keeps the permissive behaviour. The error names
  both ways out: point at an existing vault, or `vaultmind init` here.
- **`--json` failures now exit non-zero.** Every command that failed after deciding to speak JSON
  wrote an error envelope, returned an internal "already written" sentinel to avoid printing it
  twice — and then translated that sentinel into success, so the process exited **0** while the
  envelope it had just printed said `status: "error"`. `vaultmind ask --json … || handle_failure`
  therefore never fired, and any hook or script wrapping a `--json` call read success on every
  error. The sentinel now travels out to the exit code (still printing the envelope exactly once,
  on stdout, with stderr empty). Affects `ask`, `search`, `note get`/`mget`/`create`, `resolve`,
  `self`, `apply`, `doctor`, `doctor heal`, `memory links`/`neighbors`/`pack`/`related`/`summarize`,
  `frontmatter validate`, and `dataview lint`/`render`. **Scripts that (correctly) check exit
  status will start seeing failures they previously missed — that is the fix, not a regression.**
- **A brand-new vault no longer reports its own correct answer as noise.** `vaultmind init`
  scaffolds six notes and prints `ask "who am I"` as the next step; that query returned
  `identity-who-am-i` at rank 1 — exactly right — under the header
  `[relevance: weak (z=-0.11, 0.1σ below the off-topic noise floor) — body suppressed]`, so a
  user's first query on the vault we just built for them looked like a failure. Retrieval was
  correct; the *label* was wrong, because z is measured against a floor calibrated for a large
  corpus and six notes cannot clear it. The existing mitigation (the "tight vault" hint) is derived
  from a calibration snapshot needing ≥30 notes, so it was structurally unavailable to exactly the
  vaults that needed it. Below that same gate, `ask` now reports `relevance: not yet measurable —
  N notes is below the 30 needed to calibrate this vault; showing the top hit anyway` and renders
  the body instead of withholding it: a vault that small has no context budget worth protecting.
  Vaults at or above the gate are unchanged, as are confident hits and keyword-only results.
- **`capture-episode.sh` surfaces the real error when a capture fails** instead of reporting a
  generic failure, so a broken SessionEnd hook is diagnosable from its own output. (#72)

### Security
- Bump `github.com/go-git/go-git` to v5.19.2, clearing GHSA-hc8v-wwc9-vgxm. Reachable from the git
  policy checker and committer used by `note create --commit` and `apply`. (#73)

### Changed
- Dependency bumps: `go-isatty`, `ckeletin-go`, `goose`, `golang.org/x/sync`, `golang.org/x/sys`,
  `golang.org/x/text`, `modernc.org/sqlite`, and CI actions (`actions/checkout`,
  `actions/setup-go`, `goreleaser/goreleaser-action`, `github/codeql-action`). (#75)

## [0.2.3] — 2026-07-28

### Fixed
- **BGE-M3 embedding no longer hangs on a note that exceeds the model's token limit.** hugot
  defaults the tokenizer's hard truncation to the model's `max_position_embeddings` (8194 for
  BGE-M3), but XLM-RoBERTa derives position IDs with a `padding_idx+1` offset, so a sequence of
  that full length indexes *past* the position-embedding table — the ONNX
  `position_embeddings/Gather` node then wedges the forward pass (an unbounded hang on some ONNX
  Runtime builds, an out-of-bounds error on others). A dense ~12k-token note therefore stalled
  indexing at the wedge point with no output. The BGE-M3 tokenizer's own limit is now clamped to
  the embedding token budget (`BGEM3MaxTokens`, 8190) — safely below the positional ceiling — at
  pipeline construction, so no note can reach the model oversized regardless of the code path; the
  char/token pre-truncation becomes an optimization rather than the sole safeguard. (#39)

## [0.2.2] — 2026-07-27

### Fixed
- **BGE-M3 embedding no longer deadlocks partway through a vault (the "24/42 wedge").** The
  Python inference sidecar's stderr was captured but never drained during the batch loop, so a
  note past BGE-M3's 8192-token limit flooded stderr with tokenizer warnings, filled the OS
  pipe, and blocked the sidecar — and the indexer — indefinitely. stderr is now drained
  continuously into a bounded buffer; the sidecar's `Close()` joins that drain and force-kills
  an overstaying process. (#66)
- **`vaultmind index --embed` no longer silently produces a mixed-dimension index.** Re-embedding
  a bge-m3 vault (dense 1024-dim) with `--model minilm` (384-dim), or vice-versa, used to skip the
  already-embedded notes and leave a mixed index whose minority-dimension notes silently vanish
  from semantic retrieval. An incremental embed whose model dimension differs from the vault's now
  **fails closed** with an actionable message pointing at `--full` and `doctor`, and an
  unrecognized `--model` token is rejected instead of silently coercing to minilm. (#67)

### Changed
- **`vaultmind index --full --embed` now purges and re-embeds every note.** Previously `--full`
  rebuilt the content index but preserved existing embeddings, so it could not switch models and
  left mixed vaults uncorrected. It now purges all embeddings and re-embeds the whole vault as the
  requested model — which also **heals** an already-mixed vault. The purge runs only after the
  embedder loads successfully, and a `--full` run that then fails to re-embed exits non-zero
  rather than reporting success with an emptied index. A habitual `--full --embed` now pays the
  full embedding cost each run; plain `--embed` remains the cheap incremental convergence path. (#67)

### Security
- Bump `golang.org/x/text` to v0.39.0 to clear GO-2026-5970 (infinite loop on invalid input),
  reachable via Unicode normalization in the embedding and identity/envelope paths.

## [0.2.1] — 2026-07-02

### Fixed
- **Release artifacts now cross-compile for Windows.** The Contract-B custody signer's
  peer-credential check (`peerUID`, unix-only via `SO_PEERCRED`/`LOCAL_PEERCRED`) had no
  non-unix build target, so goreleaser's Windows build failed and v0.2.0 published as a Go
  module (installable via `go install`) but *without* its GitHub Release artifacts — including
  the prebuilt **ORT/BGE-M3** archives. Added a fail-closed non-unix stub so the binary
  cross-compiles; the `identity signer` daemon itself remains unix-only. v0.2.1 carries all of
  v0.2.0 plus this fix, and is the release to pull the ORT archives from.

## [0.2.0] — 2026-07-02

### Added
- **Contract-B agent identity — a full ed25519 trust-root subsystem under `vaultmind identity`.** The
  headline addition: forgery-proof agent identity for a multi-agent mesh, built in reviewed slices.
  - **Signing core + custody** — the JCS-canonical (RFC 8785) ed25519 signing core (small-order pubkey
    rejection, ZIP-215-strict verify, schema-gated signing path) and a **keyless custody signer daemon**
    (`identity signer`) that holds the private key behind a 0600 Unix-domain socket; every CLI verb is
    keyless and reaches the key only over the socket.
  - **Trust-root registry** — a root-signed, epoched, freshness-bounded `slug→pubkey` binding with
    combined revocation (monotonic epoch + `revoked-at` + fail-closed staleness), a distribution
    envelope, and a frozen cross-language fixture proving Go↔Rust byte-exact agreement.
  - **Enrollment journey** — `identity invite` (token carrying root pubkey + network id + relay),
    `identity enroll` (member self-enroll), `identity enroll-add` (admin), `identity sign-registry` /
    `sign-enrollment` / `sign-envelope`, OOB trust-anchor pinning persisted at enroll, and
    `identity --print-instructions` self-serve onboarding.
  - **Message-envelope signing** and **strict cofactorless verification** closing the Go↔Rust seam;
    plus the S3 human-principal verify-side (fail-closed authority, canonicalize parity).
- **`doctor` mesh-section** — Contract-B identity health as part of the vault health hub
  (custody / binding-resolves / chat-reachability, green only when cryptographically proven).
- **`vaultmind arc guide`** — the self-serve arc-writing discipline (the seven hunt shapes, the 5-part
  bar, the diff test, a self-pressure checklist); `arc candidates` now owns its own low recall and
  points at the guide.

### Changed
- **ORT backend honesty.** On an ORT binary, a BGE-M3 load failure now **fails loud** (naming the
  remedy) instead of silently degrading to MiniLM; `version` and `doctor` report the active backend
  (`ort+cpu` / `ort+coreml` / `go-cpu`) and give backend-appropriate remedies; a symlink-on-PATH ORT
  install is now resolved instead of silently falling back.

### Fixed
- **Indexing robustness** — cap BGE-M3 input by real token count so embedding never hangs; surface
  dropped notes and skip empty-body notes on embed; migrate `_path:`-form ids to frontmatter `id` on
  re-index with no silent data loss.
- **`doctor` honesty** — `--all` rollup totals reconcile with the per-vault reports; broken references
  are surfaced and the JSON issue-count axes are distinctly labeled; the text rollup counts the
  surfaced issue set (not the raw validation aggregate).
- **`doctor heal`** rewrites id-form wikilinks that it flags as Obsidian-incompatible.

### Security
- `golang.org/x/image` bumped to v0.43.0 (GO-2026-5061); routine CI action + dependency bumps.
- Each Contract-B slice landed with red-team / PR-review hardening (signer socket-hijack + key-perms,
  registry re-attack suites, envelope int64 widening, custody coverage floor).

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

[Unreleased]: https://github.com/peiman/vaultmind/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/peiman/vaultmind/compare/v0.2.3...v0.3.0
[0.2.3]: https://github.com/peiman/vaultmind/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/peiman/vaultmind/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/peiman/vaultmind/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/peiman/vaultmind/compare/v0.1.11...v0.2.0
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
