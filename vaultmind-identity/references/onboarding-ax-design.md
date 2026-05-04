---
id: reference-onboarding-ax-design
type: reference
title: "Onboarding AX Design — Plan for the First Real User"
created: 2026-05-02
tags:
  - onboarding
  - ax
  - new-user
  - migration
  - siavoush
related_ids:
  - arc-the-lighter-move-is-the-work
  - reference-probe-before-commit
  - reference-plasticity-priority-order
  - identity-peiman
---

# Onboarding AX Design — Plan for the First Real User

## Why this matters

Siavoush is the first real user when we are done with this work. Today's `vaultmind init` produces a clean vault scaffold (6 files, persona-shaped templates, no Peiman-personal content) but stops there. The Claude Code wiring — hooks, settings.json, path-templating — is a manual exercise. A new user gets ~30% of Peiman's experience and 70% homework. The AX gap is between "vault scaffolded" and "agent shows up integrated."

This plan captures the design conversation 2026-05-02 so we don't have to re-derive it. It is not a build plan; it is the constraint set + a probe-shaped first move.

## The constraints (Peiman, 2026-05-02)

a. One vault for a new user, not two. Don't introduce persona-vs-research split at the new-user surface.
b. The vault could be cross-project or per-project.
c. Start with Claude Code.
d. The agent should help with the install.
e. The agent should help with the scaffolding and explain what this is and what it will do for the project.
f. A new user should be able to install in an existing repo with `AGENTS.md` and `CLAUDE.md` etc. without worrying that we will wipe shit.

## Lens-walked decisions

**(a) One vault — agree, but trim the default type registry.**

Today's default registry seeds `identity, principle, arc, reference, concept, source, decision`. Concept/source are research types; decision is project-specific (ADR-flavored). For a new user the default registry should be persona-shaped only — `identity, principle, arc, reference`. Concept/source/decision can be added by the agent during install when the user's content needs them (e.g., a migration with citations).

**(b) Cross-project or per-project — yes, but persona must be SSOT.**

If a new user has both, the persona must live in exactly one of them. Two `identity-who-am-i` notes is the violation we don't introduce.

- Default: cross-project persona vault at user level (e.g., `~/.vaultmind/persona/`). One source of truth for "who I am."
- Per-project: optional domain-knowledge layer (concepts, sources, decisions). Persona stays cross-project.
- Hooks query both; persona vault wins on identity questions, project vault on domain questions.

**(c) Claude Code first.**

We dogfood it; we have 4–5 hooks shaped against its hook surface. Multi-agent abstractions before reality demands them violates probe-before-commit and principle 4 (reality is the spec).

**(d) Agent helps with install — yes, with a clean two-layer separation.**

- **CLI = deterministic primitives.** Always reproducible, idempotent, testable. Existing today: `init`, `index`, `index --embed`, `ask`, `note get`, `doctor`. To add over time as probe data tells us what's missing — likely `hooks add claude-code`, `setup-agent --check`, etc.
- **Agent = conversational surface.** Reads onboarding doc, asks user questions only the user can answer (where's the vault? who are you? what should the agent know?), invokes deterministic CLI verbs. The agent IS the installer's UI; the CLI IS the installer's engine. Free-form interpretation breaks reproducibility.

**(e) Agent explains the project specifically.**

Truth-seeking lens: agent reads the user's actual repo (`CLAUDE.md`, `AGENTS.md`, `package.json`, `README`) and summarizes what vaultmind will do for THIS specific project. Not generic marketing. "I see this is a Go CLI; vaultmind will preserve cross-session context about architectural decisions" is honest. Generic "vaultmind is the future of memory" is the bad version.

**(f) Don't wipe — three operating modes.**

- **Fresh**: no `.claude/`, no `AGENTS.md`. Write everything we own.
- **Coexisting**: existing `.claude/settings.json` and existing hooks. Merge settings (add our entries, never replace), add scripts alongside, append clearly-delineated sections to existing AGENTS.md / CLAUDE.md with comment markers so a future uninstall finds them.
- **Conflict**: hook with our name already exists OR settings.json malformed. Refuse to write, surface the conflict, offer side-path or manual merge instructions. Never destructive.

Same as today's `vaultmind init` already refuses to overwrite an existing path. Wiring code needs the same discipline.

## What the lens demands that the bullets didn't list

1. **Idempotency.** Re-running `vaultmind init` or `vaultmind hooks add` on an existing setup must be safe and predictable. Drift detection, repair mode.
2. **Diff before write.** Mutations to `.claude/settings.json` or `AGENTS.md` always print diff and require explicit confirmation. Truth-seeking applied to install: no covert writes.
3. **Reversal.** `vaultmind hooks remove claude-code` must cleanly undo what was added. If we can't uninstall, we've taken hostage of the project.
4. **The starter persona is honest about being a starter.** Today's `who-am-i.md` template has parens like "(Name, role, ...)". For human-driven init that works. For agent-led init, the agent should fill these by **interview** on first session — turning placeholders into a real first arc.

## Frontmatter primitives (reference — so we don't re-derive)

From `internal/schema/registry.go:13` — five core fields required on every domain note:

```
id, type, created, updated, vm_updated
```

- `id` — unique vault-wide identifier
- `type` — must be registered in `.vaultmind/config.yaml`
- `created` — creation date (e.g., `2026-05-02`)
- `updated` — human-managed timestamp (Obsidian-style)
- `vm_updated` — vaultmind-managed timestamp

Plus type-specific required fields:
- Most persona types: `title`
- `source`: `title, url`
- `decision`: `title, status`

**Graph-tier fields** (recognized on any type, optional, used by retrieval):
```
title, status, aliases, tags, parent_id, related_ids, source_ids
```

**Domain-note classification** (per `parser/frontmatter.go:79`): a file is treated as a domain note only if both `id` AND `type` are populated. Files without those two are tolerated in the vault directory but not indexed. So `README.md`, `_index.md`, etc. without frontmatter sit harmlessly in the vault — useful coexistence property.

**Heterogeneous types coexist cleanly** if:
- Every type is in the registry.
- Every note has core + type-required fields.
- Subdirectory convention is followed (recommended; not enforced).

The two existing vaults prove this — `vaultmind-vault` (407 notes spanning concept/source/decision/...) and `vaultmind-identity` (33 spanning identity/principle/arc/reference) both work.

## Two onboarding paths

The agent's first action after preflight is "what does this project look like?" If `*.md` files with content already exist, propose Path 2 (or hybrid). Otherwise Path 1.

### Path 1 — Greenfield

User has no existing knowledge base. Agent walks them through:
1. `vaultmind init <path>` (default: `~/.vaultmind/persona/`).
2. Interview-driven identity (turn parens placeholders into a first real identity note + a first arc).
3. `.claude/settings.json` + hooks (diff-preview before write).
4. `vaultmind doctor` to verify green.
5. Trigger a sample retrieval to demonstrate.

### Path 2 — Migration

User has existing markdown content (with or without frontmatter). Agent:
1. Surveys content (sample files, identifies conventions, infers types).
2. Proposes a registry that fits the existing content.
3. Generates and **adds** vaultmind core fields to each file (diff-preview, file-by-file or batch with confirmation). Doesn't move files (preserves git history). Doesn't change content.
4. Maps overlapping fields where possible (e.g., Obsidian `tags` → vaultmind `tags` is identical key, no-op).
5. **Keeps user-specific custom fields untouched.**
6. Writes `.vaultmind/config.yaml`, indexes, samples retrieval.

Frontmatter is **additive** at the migration layer. No content rewritten, no fields stripped.

## The probe — content-machine

We have a real test corpus: `/Users/peiman/dev/daana/daana-content-machine/`.

Layout (peeked 2026-05-02):
- `knowledge_base/` with subdirectories like `data_engineering/`, `data_architecture/`.
- Files like `principles.md`, `patterns.md`, `anti_patterns.md`, `best_practices.md` — domain content.
- `_index.md` files as router/summary nodes (no frontmatter — pure markdown).
- Multiple top-level dirs: `archive/`, `content/`, `docs/`, `published/`, `style_guide/`, `templates/`.

**Sample read**: `knowledge_base/_index.md` has no frontmatter. So content-machine is largely **frontmatter-free** — the migration shape is "ADD frontmatter," the simpler case. (Users with Obsidian-shaped frontmatter are the harder case; defer until we have one.)

### Update 2026-05-02 — corrected probe numbers

The "largely frontmatter-free" claim above was an overgeneralization from 5 sampled files. Tightened test (`---` must be on **line 1**, not anywhere) gives the truth: **56 of 393 .md files** in content-machine have actual line-1 frontmatter (~14%). The 261 number from a looser grep was inflated by `---` matches as horizontal-rule dividers.

The 14% with frontmatter use a Diátaxis-shaped dialect:
```
title, summary, audience, status, last_verified, related
```
Concentrated in `docs/`, `.claude/agents/`, parts of `style_guide/` (e.g., `style_guide/DESIGN.md` is YAML config-as-markdown).

The 86% without frontmatter need full vaultmind core fields added.

**Multiple dialects** in content-machine (Diátaxis docs vs. component descriptors vs. nothing) make it the harder migration test. The right first probe is shahname-rts (uniform dialect, ~26 files, mostly-complete frontmatter).

**Probe protocol** (informs the onboarding doc, not the other way around):

1. Read 5–10 representative files from content-machine — sample across `data_engineering/`, `data_architecture/`, `style_guide/`, etc.
2. Infer the type registry the content needs. Likely: `concept` for most domain content, `principle` for principles.md-style files, possibly `reference` for index files. Probably no `arc` — content-machine isn't transformation-shaped.
3. Draft the frontmatter the agent would add to ONE file. Verify it round-trips through vaultmind validators.
4. Identify what the agent had to **figure out vs. ask the user** — directory-name-as-category? id-naming convention? whether `_index.md` files should be indexed?
5. Each thing the agent had to figure out without help → instruction line in the onboarding doc.
6. Each thing the agent had to ask → an interview prompt in the onboarding doc.

The probe DRIVES the doc design. We don't pre-design the doc; we run the migration once, watch what's load-bearing, write the doc from observation.

## Probe results — shahname-rts (2026-05-02)

Test corpus: `/Users/peiman/dev/siavoush/shahname-rts/` (Siavoush's RTS-game project). 26 of 28 .md files have line-1 frontmatter, all in a single consistent dialect.

**Type vocabulary in use** (8 distinct types):
```
contract:5  plan:3  log:3  research:2  process:2  spec:1  manifesto:1  architecture:1
```
None map naturally to vaultmind's default registry. They carry domain meaning specific to game design and project-process work. Adopt their types into the registry — don't flatten to vaultmind's persona vocabulary. This validates the design decision: vaultmind's default registry is a **starting suggestion**, not a fixed schema.

**Field inventory** (15 distinct fields):
```
audience, created, last_updated, owner, prerequisites, provenance,
read_when, references, ssot_for, status, summary, tags, title, type, version
```

**Mapping to vaultmind contract**:
| shahname-rts | vaultmind | action |
|---|---|---|
| `created` | core `created` | ✅ identical |
| `last_updated` | core `updated` | **NEEDS ALIASING** |
| (missing) | core `vm_updated` | add per file |
| (missing) | core `id` | add per file |
| `title, tags, status` | graph fields | ✅ identical |
| `references` | graph `related_ids` (different content shape — paths vs ids) | preserve as-is, defer mapping |
| `audience, owner, summary, read_when, prerequisites, ssot_for, provenance, version` | type-specific custom | ✅ pass through, vaultmind tolerates |

**Migration cost per file**: 2 fields added (`id` + `vm_updated`). Body untouched. Existing 13 fields untouched. **~52 lines across 26 files** total. The migration is genuinely trivial *if* aliasing lands first.

**What the agent figured out vs. asked** (informs onboarding doc):

Figured out (mechanical):
- Type vocabulary via `grep -h "^type:" *.md | sort | uniq -c`.
- Field inventory via `head -30` across files.
- Mapping table is mechanical given a vocabulary diff.
- File-level granularity is appropriate (these are well-scoped docs, not multi-concept files).

Must ask user:
- Confirm adopt-types vs. remap (default: adopt).
- Confirm `last_updated` → `updated` aliasing (vs. rename in-file).
- ID-naming pattern: type-prefix-slug from filename (`research-shahnameh-rts`)?
- Whether `references: [paths]` should eventually convert to vaultmind ids (defer).

## Field aliasing — required now (was deferred)

Authorized 2026-05-02. Necessary for migrations that respect existing user vocabulary.

**Slice plan**:
1. Extend config schema (`internal/vault/config.go` or wherever `Config` lives): add `Schema.Aliases map[string][]string`.
2. Update validator (`internal/schema/registry.go`): `RequiredFields` and `Validate` accept any registered alias when canonical name is missing.
3. **TDD tests**:
   - Note with `last_updated` (no `updated`) + alias config → passes validation.
   - Note with `updated` directly → still passes (canonical wins).
   - Note with neither → fails (missing required).
   - Multiple aliases registered → first-found wins, deterministic.
4. **Non-destructive on read.** Aliases are recognized at validation, not normalized. The file keeps `last_updated`; the in-memory representation accepts it where `updated` is required.
5. Documentation in init template's `config.yaml` shows aliases as a config option.

Estimated complexity: small-medium (~2-3 hours focused). Atomic commit (test + impl + docs).

## One vault for cross-repo — feasibility analysis

Hypothetical asked 2026-05-02: could one vault hold both content-machine AND shahname-rts?

**Yes, mechanically.** Today's `.vaultmind/config.yaml` supports it. Architecture:

```
~/vaultmind-mega/
├── .vaultmind/
│   ├── config.yaml      # UNION type registry (16+ types)
│   └── index.db
├── cm/                  # → symlink to daana-content-machine
└── shahname/            # → symlink to shahname-rts
```

What works out of the box:
- Per-subdir exclusion via `vault.exclude` for both repos' `.git` etc.
- Heterogeneous types in one registry — vaultmind already supports this.
- `.vaultmind/` lives at the shared root, doesn't pollute either repo's git.

What the user must manage by convention (vaultmind doesn't enforce):
- **ID namespace discipline.** Vaultmind enforces vault-wide unique ids. If both repos generate `plan-foo`, collision. Convention: prefix `cm-*` and `shahname-*`.
- **Type collisions.** If `plan` exists in both repos with different field requirements, the global registry is the union. No per-subdir type scoping today.
- **Embedding-space mixing.** RTS lore + data engineering principles in one BGE-M3 index. Cross-domain retrieval works (sometimes a feature: cross-pollination across projects; sometimes a bug: a Shahnameh research note answering a data architecture query).

**Honest read on shape**: yes possible, but not the natural production shape. The natural shape:
- One persona vault (cross-project, user-level).
- Per-project knowledge vault (per repo).
- Optional cross-vault federated retrieval (Paper #2 territory) for queries that span projects.

The mega-vault is a **research capability** — useful for Paper #2's federated tuning evidence, not the default new-user setup. Worth keeping as a documented topology for advanced users / research, not a recommended one for Siavoush.

## Doctor extension (deferred)

`vaultmind doctor` today is vault-scoped (schema, frontmatter, broken links, embedding coverage, citations). Hook health is missing. New-user risk: green vault + broken hooks = silent failure.

Cleanest extension: when run inside a directory whose CWD or any parent has `.claude/settings.json`, doctor adds a "Project integration" section that checks:
- Hooks file parses; matchers cover the expected lifecycle points.
- Each hook command points to a script that exists and is executable.
- Each script's `vaultmind` invocation references a binary that's reachable.
- Smoke tests for each hook pass.
- Sidecar logs in `~/.vaultmind/*` show recent invocations (proof of life).

Outside a Claude Code project: section silently absent. No coupling at the abstract layer; opportunistic detection.

**Defer until after the content-machine probe** — probe data tells us which checks are load-bearing for real failure modes. Don't pre-build.

## What we explicitly defer

- **New CLI verbs** (`vaultmind hooks add claude-code`, `vaultmind setup-agent`). Agent can do these writes via `Edit/Write/Bash` following the doc's instructions for the first probe. Reality (Siavoush's actual experience) tells us which writes deserve promotion to deterministic CLI verbs.
- **Multi-agent adapters** (Cursor, Codex, etc.). Wait until reality demands.
- **One-shot uninstall command.** Need diff-before-write discipline so manual uninstall is "delete the markered sections," but don't have to ship `vaultmind uninstall` today.

## Open design questions

1. **Default cross-project vault path.** `~/.vaultmind/persona/` vs `~/Library/Application Support/vaultmind/persona/` (macOS) vs XDG-shape on Linux. Consider what `vaultmind config` does today.
2. **Where the onboarding doc lives.** In the vaultmind repo? Embedded in the binary? Pulled from a URL? How does the agent FIND it on first session?
3. **First-session entry point.** What does the user paste/type into Claude Code to trigger the onboarding? A single sentence pointing at the doc, or a slash command, or something else?
4. **Hybrid Path 1 + Path 2** for users who want a fresh persona vault AND migrate existing project content. Likely: persona vault at user level, project vault co-located with the project. Agent does both flows in sequence.

## Source

- Session date: 2026-05-02.
- Conversation transcript: `~/.claude/projects/-Users-peiman-dev-cli-vaultmind/<this-session-id>.jsonl` (auto-captured at SessionEnd to `vaultmind-identity/episodes/episode-2026-05-02-*.md`).
- Verbatim Peiman framings findable in transcript:
  - "now I want to talk about how a new user could start from scratch using vaultmind. I want to have the scaffold and make a good start for them we need to think that it should be AI first great AX (AI Experience) but not have all my personal files and shit"
  - The six-bullet constraint set (a–f above) given in a single message.
  - "Siavoush will be the first real user when we are done with this."
  - "we could use that as a probe for instructions to the AI agent."
- Companion arc: `arc-the-lighter-move-is-the-work` (the discipline this plan honors — probe-before-commit at scope, ship the lightest probe first, don't pre-design).
- Companion reference: `reference-probe-before-commit` (the principle this plan extends).
- Test corpus: `/Users/peiman/dev/daana/daana-content-machine/`.
