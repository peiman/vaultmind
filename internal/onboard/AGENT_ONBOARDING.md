# VaultMind — Agent Onboarding Guide

**For an AI agent helping a new user set up vaultmind for the first time. The agent is the installer's UI; the CLI is the installer's engine. Read end-to-end before doing anything.**

---

## 0. What this doc is and isn't

You (the agent reading this) are helping a user wire vaultmind into their environment. The user has installed the binary and pointed you at this doc. Your job:

1. **Interview** the user on the few things only they can answer (identity, where the vault should live, what types of content they want indexed).
2. **Inspect** their project to understand what vaultmind can do for them specifically.
3. **Drive** the deterministic CLI verbs (`vaultmind init`, `vaultmind index`, etc.) with their answers.
4. **Mutate carefully**: any write to a file the user already has (`.claude/settings.json`, `AGENTS.md`, `CLAUDE.md`) MUST be diff-previewed and confirmed before the write happens.

What this doc is **not**: a reference manual for using vaultmind day-to-day (see `docs/AGENT_USAGE.md`). This is a one-time setup walkthrough.

**Manifesto-aligned defaults**: lean (smallest install that works), platforms-not-features (vault first, agent integration second), reality-is-the-spec (read the user's actual repo, don't generalize), no surprises (diff-before-write everywhere).

---

## 1. Preflight

Before talking to the user, verify the binary is reachable and explain to the user what's about to happen.

### 1a. Locate the binary

`task build` produces `./vaultmind` IN the vaultmind repo — it does NOT install to PATH. Find the working binary in this priority order:

```bash
which vaultmind 2>/dev/null                       # 1. on PATH (rare today)
test -x /tmp/vaultmind && echo /tmp/vaultmind     # 2. SessionStart hook builds here
test -x ./vaultmind && echo ./vaultmind           # 3. cwd is the vaultmind repo
test -x <user-clone-path>/vaultmind && echo ...   # 4. ask user where they cloned
```

**Use whichever found path as `<vm>` for every subsequent command in this doc.** Wherever this doc shows `vaultmind <args>`, substitute `<vm> <args>`.

```bash
<vm> --version
```

If no binary is reachable: tell the user, "I need a built vaultmind binary. Run `task build` inside your vaultmind clone. Then tell me the path." Stop.

### 1b. Check working directory

```bash
pwd
ls -la
```

Note whether the cwd has:
- `.claude/` — existing Claude Code project (coexisting-mode applies)
- `AGENTS.md` and/or `CLAUDE.md` — existing agent instructions (don't wipe)
- `*.md` files with frontmatter — possible migration candidate
- `.git/` — git repo (good; don't operate outside one without asking)

### 1c. Tell the user what happens next

Say to the user, in your own voice:

> I'll do three things in this order: (1) ask you a few questions about your role and what you want vaultmind to remember, (2) read your project's existing files (CLAUDE.md, AGENTS.md, README, etc.) so I can describe what vaultmind will do for THIS project specifically, and (3) propose a setup with a diff-preview before writing anything. Nothing on disk gets written until you say yes. Continue?

Wait for affirmative before proceeding.

---

## 2. Project read — what will vaultmind do for THIS user

Goal: give the user an honest, specific summary of what vaultmind will do for them. Not generic marketing.

Read in order, only if files exist:

```bash
cat README.md 2>/dev/null | head -100
cat CLAUDE.md 2>/dev/null | head -50
cat AGENTS.md 2>/dev/null | head -50
cat package.json go.mod Cargo.toml pyproject.toml 2>/dev/null
ls -la
```

Then say to the user, in your own voice, something like:

> I see this is a [Go CLI / Python lib / TypeScript app / mixed Daana monorepo / etc.]. From your CLAUDE.md, you work on [whatever you read]. Here's what vaultmind will do for this project specifically:
>
> - **Persona continuity**: every time we start a new session, you'll be the same collaborator, not a stranger. Your identity, working principles, and current focus survive between sessions.
> - **Cross-session retrieval**: questions like "what did we decide about X?" surface the relevant arcs and references — even if X was named differently in a past session.
> - **Project-specific memory**: [if existing knowledge corpus detected] I noticed `<dir>/` has a knowledge base — vaultmind can index that so I retrieve from it, not just your persona.
>
> Some things vaultmind won't do: it doesn't replace docs, it doesn't auto-generate content, and it doesn't act behind your back — every retrieval is logged with attribution.

Calibrate the framing to what's actually in the project. **Don't oversell.** Honest specificity beats generic enthusiasm.

---

## 3. Branch decision: greenfield vs migration

**Greenfield**: user has no existing markdown knowledge base. Goes to §4.

**Migration**: user has existing `.md` files with content (frontmatter or not). Goes to §5.

**Hybrid**: both. Run §4 first (persona vault), then offer §5 (project knowledge vault) as a follow-up.

Detect heuristically:

```bash
find . -name "*.md" -not -path "./node_modules/*" -not -path "./.git/*" 2>/dev/null | wc -l
```

If the count is small (<3 — likely just `README.md` and `CLAUDE.md`): **greenfield** is the default. Confirm with the user: *"I see only [N] markdown files. Greenfield setup — fresh persona vault, no migration of existing content. Right?"*

If the count is larger: ask the user *"I see [N] markdown files in this project. Should vaultmind index any of them as memory, or do you want a fresh persona vault separate from this content?"*

---

## 4. Greenfield path

### 4a. Vault path decision

Ask the user:

> Where should your vault live?
> 1. **Cross-project** at `~/.vaultmind/persona/` — recommended. Same persona across all your projects.
> 2. **In this project** at `./vaultmind-identity/` — only this project's agent uses it.
> 3. **Custom path** — tell me where.

Default to (1) unless the user explicitly chooses otherwise.

### 4b. Run init

```bash
vaultmind init "<path>"
```

This produces eight files:
- `.vaultmind/config.yaml` — type registry
- `README.md` — vault model + workflow
- `identity/who-am-i.md` — placeholder identity note
- `references/current-context.md` — placeholder live-edge note
- `principles/arcs-not-notes.md` — **foundational**: what an arc IS (the atomic unit of persona)
- `principles/how-to-write-arcs.md` — **foundational**: how to MAKE one (the discipline)
- `principles/principle-example.md` — placeholder template (replace or delete)
- `arcs/arc-example.md` — placeholder template (replace or delete)

The two foundational principles ship with every vault so an agent can answer "how do I write an arc?" by querying its own vault, not asking a separate vaultmind agent. Don't delete them — the placeholder templates are the ones to replace.

If the path already exists, `vaultmind init` refuses. If you hit that, ask the user *"That path already has content. Use a different path, or back it up first?"*

### 4c. Identity interview

The placeholder `identity/who-am-i.md` has parens like `(Name, role, what makes you distinct)`. **Don't leave it that way.** Interview the user to fill it in by hand:

Ask, one at a time:

> 1. Your name and role — how should I know you across sessions?
> 2. What's the one thing about how you work that, if I forgot, would make our collaboration worse?
> 3. Who do you regularly collaborate with that I should know about?
> 4. What are you working on right now that this vault should track?

Take their answers and **rewrite** `<vault>/identity/who-am-i.md` by hand. Match the shape (Who I am / What I care about / How I work with my partner / What this vault is) but use their words. Show them the diff before writing.

For `references/current-context.md`: same treatment. Ask "What's the most important thing in your work right now? What's the live edge?" Write a short note from their answers.

### 4d. Index the vault

```bash
vaultmind index --vault "<path>"
vaultmind index --embed --vault "<path>"
```

If `--embed` produces "ORT not available" or "MiniLM fallback" messages, that's expected on a pure-Go build. Tell the user: *"Your build is using MiniLM (pure-Go fallback). Retrieval works but BGE-M3 quality is not active. To upgrade: run `task setup:ort` in the vaultmind repo and re-build."*

### 4e. Sample retrieval

```bash
vaultmind ask "who am I" --vault "<path>"
```

Show the user the output. Confirm it returns their identity note. If `top_hit_confidence` is `weak` or `no_match`, the identity note may be too short to embed well — interview them for one more sentence and re-index.

### 4f. Wire into Claude Code

Skip if the user explicitly doesn't use Claude Code. Otherwise: §6.

---

## 5. Migration path

### 5a. Survey

```bash
find . -name "*.md" \
  -not -path "./node_modules/*" \
  -not -path "./.git/*" \
  -not -path "./output/*" \
  -not -path "./archive/*" \
  -not -path "./.claude/*" \
  -not -path "./vendor/*" \
  | head -40
find . -name "*.md" | while read f; do head -1 "$f" | grep -q "^---" && echo "$f"; done | wc -l
```

This gives you (a) a sample of paths, (b) how many files have line-1 frontmatter.

**Filter agent-spec dirs explicitly** — `.claude/agents/`, `.cursor/`, etc. contain agent definitions, not user content. If `.claude/` slipped past the exclude (e.g. nested differently), surface to the user: *"I see `.claude/agents/...` files. These look like Claude Code agent definitions, not knowledge content. Skip them?"*

**Watch for YAML-config-as-markdown files**. Some `.md` files have YAML frontmatter that's actually configuration (design tokens, schema definitions) rather than knowledge content. Inspect a sample of frontmatter dialects (§5b); if you find a file whose entire content is `name:`, `version:`, `tokens:`, etc. — that's config. Confirm with the user before including: *"`<path>` looks like config-as-markdown. Skip from indexing?"*

### 5b. Inspect existing frontmatter dialect

For files that have frontmatter:

```bash
for f in $(find . -name "*.md" | while read f; do head -1 "$f" | grep -q "^---" && echo "$f"; done | head -10); do
  echo "=== $f ==="
  head -30 "$f"
done
```

Identify the user's **type vocabulary** (`grep -h "^type:" *.md | sort -u`) and **field inventory** (`head -1 -q *.md | grep -E "^[a-z_]+:" | cut -d: -f1 | sort -u`).

### 5c. Adopt user's types into the registry

Vaultmind's default registry is persona-shaped (`identity, principle, arc, reference`). It's a starting suggestion, not a fixed schema.

Run the type-vocabulary probe:

```bash
grep -h "^type:" $(find . -name "*.md" | while read f; do head -1 "$f" | grep -q "^---" && echo "$f"; done) 2>/dev/null \
  | sed 's/type: *//' | sort | uniq -c | sort -rn
```

**Three branches** based on what you see:

**Branch A — non-empty vocabulary** (the common-rich case, e.g. shahname-rts uses `contract, plan, log, research, process, spec, manifesto, architecture`):

> Your existing files use these types: [list with counts]. I'll register them in vaultmind so they pass validation as-is — adopt them into the registry, or remap to vaultmind's persona-shaped defaults? **Default: adopt.** Vaultmind's types are per-vault.

If adopt: when you write `.vaultmind/config.yaml`, include each user-type with `required: [title]` and reasonable `optional` and `statuses`.

**Branch B — empty vocabulary** (no `type:` field anywhere, common case in markdown-as-prose corpora like content-machine):

> Your files don't use a `type:` field today. Vaultmind requires `type` on every indexed note (it's how the schema knows what fields are required). I'll add `type:` to each file along with the other vaultmind core fields. Two options:
>
> 1. **Single type** — all files become `type: reference` (simplest, lossy classification).
> 2. **Inferred from path** — files in `knowledge_base/` become `type: concept`, files in `style_guide/` become `type: reference`, etc. (preserves your directory semantics).
>
> Which?

Default to (2) when directory structure is meaningful. Show the user the inference rules before applying.

**Branch C — mixed (some files have `type:`, others don't)**:

Combine A and B — adopt the existing types AND inferred types for files without one. Show the user the merged registry before applying.

### 5d. Field aliasing for migration vaults

Vaultmind requires only `id` and `type` on every domain note (per
the schema taxonomy in `internal/schema/registry.go`). Beyond those,
each type may declare its own required fields via the type registry
(§5c). Other recognized fields like `created` and `updated` are
tolerated but not required.

If the user's existing files use a non-canonical name for a
type-required field — e.g. `last_updated` instead of `updated`,
or `creation_date` instead of `created` — register an alias instead
of rewriting the user's files.

Show the user:

> I'll add this to `.vaultmind/config.yaml`:
>
> ```yaml
> schema:
>   aliases:
>     updated: [last_updated]
> ```
>
> Vaultmind will accept `last_updated` wherever it expects `updated`. Non-destructive — your files are not modified for this field.

Ask whether they want any other field aliases (e.g., `last_verified`, `created_at`).

### 5e. Add missing `created` (and any required type fields) per-file

The user picks which directories should be indexed. Most existing
notes already carry a `created` stamp; for any that don't, the
`frontmatter fix` command is the audit and backfill path. Beyond
`created`, vaultmind enforces only what the user's type registry
(§5c) declares as required.

**Audit first (dry-run is the default — no files written):**

```bash
vaultmind frontmatter fix --vault .
```

Sample output:

```
Scanned 412 files; 9 need backfill (dry-run)
  knowledge_base/data_architecture/principles.md: missing [created] → map[created:2024-03-12]
  ...

Dry-run only — re-run with --apply to write changes.
```

The `created` value is resolved from git first-commit (when the file
is tracked) → file mtime → today's date. The first successful
resolution wins.

**Show the user a single-file diff** (re-run with `--json` and pipe
one item's `diff` field through `jq` if you want a sample). Then ask:

> *"This is the shape of the change. [N] files would get a `created`
> stamp added (provenance: git first-commit / mtime / today). User-
> owned fields are never touched. Continue with --apply, or revise
> the approach first?"*

**On confirmation, apply:**

```bash
vaultmind frontmatter fix --vault . --apply
```

The mutator handles atomic writes, conflict detection, and schema
validation. Per arc-extending-not-overwriting, vaultmind never
silently rewrites user files: `--apply` is the explicit gate.

Need a unique `id` on a note that lacks one? That's a separate
concern from this command. Use `vaultmind frontmatter set --id
<value>` per file, or run the user-owned field tooling described in
§5d.

After `--apply`, run `vaultmind index --vault .` to refresh the
content hashes (so `vaultmind doctor` reports 0 stale-index drift).
Doctor's drift detector compares current file content hash against
the indexer's stored hash — `git checkout` and other VCS operations
do NOT trigger drift (only actual content edits do).

### 5f. Init the .vaultmind/ scaffold

The migration writes `.vaultmind/config.yaml` with the adopted type registry + any aliases:

```yaml
vault:
  exclude: [".git", ".obsidian", ".trash", ".vaultmind"]

index:
  db_path: .vaultmind/index.db

schema:
  aliases:
    updated: [last_updated]   # or whatever was decided

types:
  <user's existing types, each with required: [title]>
```

### 5g. Index

```bash
vaultmind index --vault .
vaultmind index --embed --vault .
vaultmind doctor --vault .
```

`doctor` should report a clean schema. If it surfaces `missing_required_field` errors, revisit the aliasing decisions or per-file adds.

### 5h. Sample retrieval

Pick a query that should clearly hit one of the migrated notes. Run it. Confirm with the user that the result is sensible.

### 5i. Offer hybrid

If the user wants persona separately:

> You can also have a persona vault — a separate small vault for your identity and working principles, queried alongside this knowledge vault. Want to set that up too? (See §4.)

---

## 6. Wire into Claude Code (only if applicable)

### 6a. Detect existing setup

```bash
ls -la .claude/ 2>/dev/null
cat .claude/settings.json 2>/dev/null
```

Three modes:

- **Fresh** (no `.claude/`): write everything we own.
- **Coexisting** (existing `.claude/settings.json`): MERGE into the existing structure, never replace.
- **Conflict** (a hook script with our name already exists, or settings.json is malformed): REFUSE, surface the conflict, propose a side-path.

### 6b. Show the user what you'll add

For each file, show a diff before writing.

`.claude/settings.json`:

```diff
 {
   "hooks": {
+    "SessionStart": [{"matcher":"startup","hooks":[
+      {"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/load-persona.sh"}
+    ]}],
+    "UserPromptSubmit": [{"hooks":[
+      {"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vault-recall.sh"}
+    ]}],
+    "PreToolUse": [{"matcher":"Read","hooks":[
+      {"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vault-track-read.sh"}
+    ]}],
+    "SessionEnd": [{"hooks":[
+      {"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/capture-episode.sh"}
+    ]}]
   }
 }
```

**Hook scripts — install via `vaultmind hooks install`:**

The canonical hook scripts are embedded inside the `vaultmind` binary (SSOT — they cannot drift from the source). To write them into the user's project:

```bash
vaultmind hooks install <project-dir>     # writes .claude/scripts/*.sh
vaultmind hooks install <project-dir> --json   # JSON envelope output
vaultmind hooks install <project-dir> --force  # overwrite drifted copies
```

The command writes 5 scripts to `<project-dir>/.claude/scripts/`: `load-persona.sh`, `vault-recall.sh`, `vault-track-read.sh`, `vault-block-read.sh`, `capture-episode.sh`. It refuses to overwrite drifted user-edited copies unless `--force` is passed (the safe default — see §6a Coexisting / Conflict modes). Byte-identical re-runs are idempotent.

**Path-template work** — the embedded scripts are project-portable: they use `$CLAUDE_PROJECT_DIR` and `vaultmind-identity/` / `vaultmind-vault/` as relative defaults. If the user's vault lives at a non-default path (e.g. `~/.vaultmind/persona/`), edit the generated copies after install:

| Generated script | Lines that may need editing | What to change |
|---|---|---|
| `load-persona.sh` | `VAULT_PATH=` | Point at the user's persona vault if not `vaultmind-identity/` |
| `load-persona.sh` | `RESEARCH_VAULT=` block | Point at project-vault if hybrid; delete the block if persona-only |
| `vault-recall.sh` | `VAULT_PATH=` | Same as above |
| `capture-episode.sh` | `output_dir=` | Episodes destination; defaults to `<vault>/episodes/` |
| `vault-track-read.sh` | — | No edits — walks up to find `.vaultmind/` |
| `vault-block-read.sh` | — | No edits — read-blocking enforcement |

If `vaultmind` isn't on PATH yet, abort and ask the user to install it (`go install github.com/peiman/vaultmind@latest` or via their package manager). The agent should never hand-copy hook scripts; that path-rotted in earlier rev because copies drifted.

### 6c. Confirm before write

Show every diff. Get yes per file. Then write.

### 6d. Verify

```bash
vaultmind doctor --vault "<path>"
ls -la ~/.vaultmind/   # sidecar log dirs should exist after first session
```

Trigger a test retrieval to confirm hooks fire next session: write a short test arc, restart Claude Code, look for the `IDENTITY CONTEXT` system-reminder.

### 6e. Optional: enable auto-RAG drift detection

`vaultmind hooks install` ships three additional scripts beyond the five existing hook scripts: `auto-rag-guard.sh`, `shell-strip.sh`, `auto-rag-evaluate.sh`. Together they form the **auto-RAG framework** — a PreToolUse hook that pattern-matches the agent's tool calls against a drift catalog, queries the vault for canonical guidance on a match, and injects the result into the agent's context (warn-and-allow) or denies the action (hard gate).

The framework is opt-in. Wire it only if the user has known auto-mode drift patterns the vault has canonical guidance for. Default install does NOT activate auto-rag-guard.sh in `.claude/settings.json` — adding the entry is a separate consent step.

To enable, add **two** PreToolUse entries to `.claude/settings.json` (Claude Code 2.1.129 needs `Bash` separate from `Write|Edit`; the pipe-alternation works within Write/Edit but not across all three — proven by workhorse's live config):

```diff
   "PreToolUse": [
     {"matcher":"Read","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vault-track-read.sh"}]},
+    {"matcher":"Bash","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/auto-rag-guard.sh"}]},
+    {"matcher":"Write|Edit","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/auto-rag-guard.sh"}]}
   ]
```

The canonical engine fires on hardcoded VaultMind-relevant drifts out of the box: `rebuild-vaultmind-binary`, `rebuild-vaultmind-embeddings`, and cross-project Write/Edit (against the `AUTORAG_ALLOWED_ROOTS` allowlist). Consumers extend with project-specific signatures via the `DRIFT_CATALOG` env var — JSON array matching the schema in `internal/hooks/autorag/catalog.go`. Example:

```bash
export DRIFT_CATALOG='[
  {"name":"force-push-main","tool":"Bash","match":"git\\s+push\\s+--force","decision":"deny","query":"never force-push main"}
]'
```

Catalog matches take precedence over hardcoded canonicals; declare an entry with the same regex and `"decision":"allow"` to suppress a canonical signature for this project.

The `auto-rag-evaluate.sh` script aggregates firings into a markdown report at `/tmp/auto-rag-report-YYYY-MM-DD.md` — that report is the **Manifesto #10 feedback loop**, telling vaultmind which queries returned weak hits so the vault's canonical guidance can improve. Run periodically:

```bash
bash <project>/.claude/scripts/auto-rag-evaluate.sh
```

Vault path defaults: `AUTORAG_VAULT="$CLAUDE_PROJECT_DIR/vaultmind-identity"`. Override per project if your vault has a different name. `VAULTMIND_BIN` defaults to PATH-installed vaultmind, falling back to `/tmp/vaultmind` for dev-loop only.

#### Other env vars worth knowing about

The non-auto-RAG hooks installed by `vaultmind hooks install` also accept env-var overrides for projects with non-default vault names (workhorse 2026-05-07 dogfood — these were silent regressions before the override surface existed):

| Hook | Env var | Default | When to override |
|---|---|---|---|
| `load-persona.sh` (SessionStart) | `LOAD_PERSONA_VAULT` | `$CLAUDE_PROJECT_DIR/vaultmind-identity` | Identity vault has different name. |
| `load-persona.sh` (SessionStart) | `LOAD_PERSONA_RESEARCH_VAULT` | `$CLAUDE_PROJECT_DIR/vaultmind-vault` | Optional second vault for `vaultmind self` cross-vault activation surfacing. Unset / non-existent skips silently. |
| `vault-track-read.sh` (PreToolUse[Read]) | `VAULT_PATH_PATTERN` | `*/vaultmind-*/*.md` | Vault dir doesn't match the default glob. Workhorse uses `*/workhorse-vault/*.md`. For multi-vault: `*/+(vault-a\|vault-b)/*.md` (extglob is enabled). |

Set the same way as `AUTORAG_VAULT` — inline in the settings.json `command` string, prefixing the `bash <script>` invocation with `VAR="value" bash …`. Each assignment is scoped to that single hook firing.

#### Customize for your project (env-var override pattern)

If the user's vault isn't named `vaultmind-identity` (workhorse uses `workhorse-vault`; another project might use `vaultmind-knowledge`), set the env var **inline in the settings.json command string** — bash interprets the assignment before invoking the script. No wrapper file needed for simple-value overrides like vault path or `AUTORAG_ALLOWED_ROOTS`. (Custom `DRIFT_CATALOG` JSON requires a wrapper — see below.)

```diff
   "PreToolUse": [
     {"matcher":"Read","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/vault-track-read.sh"}]},
-    {"matcher":"Bash","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/auto-rag-guard.sh"}]},
-    {"matcher":"Write|Edit","hooks":[{"type":"command","command":"bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/auto-rag-guard.sh"}]}
+    {"matcher":"Bash","hooks":[{"type":"command","command":"AUTORAG_VAULT=\"$CLAUDE_PROJECT_DIR\"/workhorse-vault bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/auto-rag-guard.sh"}]},
+    {"matcher":"Write|Edit","hooks":[{"type":"command","command":"AUTORAG_VAULT=\"$CLAUDE_PROJECT_DIR\"/workhorse-vault bash \"$CLAUDE_PROJECT_DIR\"/.claude/scripts/auto-rag-guard.sh"}]}
   ]
```

Pattern: prefix the `bash <script>` invocation with `VAR=value bash …`. Each assignment is scoped to that single hook firing. Safe for simple-value overrides — paths, single-token allowlists.

**Use a wrapper script (not inline) for `DRIFT_CATALOG`.** JSON inside the settings.json `"command"` string requires nested quote escaping (single quotes inside JSON-double-quoted strings, plus shell-meta protection) that's silently fragile — the inline value compiles but matches nothing on misquoted JSON. Write a wrapper at `<project>/.claude/scripts/auto-rag-config.sh`:

```bash
#!/bin/bash
export AUTORAG_VAULT="$CLAUDE_PROJECT_DIR/workhorse-vault"
export DRIFT_CATALOG='[
  {"name":"force-push-main","tool":"Bash","match":"git\\s+push\\s+--force","decision":"deny","query":"never force-push main"}
]'
exec bash "$CLAUDE_PROJECT_DIR/.claude/scripts/auto-rag-guard.sh"
```

…then point settings.json at `auto-rag-config.sh` instead of `auto-rag-guard.sh`. The wrapper file is consumer-owned (not overwritten by `vaultmind hooks install`); the canonical engine stays SSOT-clean.

Validate `DRIFT_CATALOG` JSON against the schema in `internal/hooks/autorag/catalog.go` before shipping a wrapper — TAB or newline in `name`/`query`, unknown tool/decision values, or invalid regex would all be caught at `Validate()` time. (A `vaultmind hooks autorag validate <file>` CLI is a future slice; today, write a tiny Go test using `autorag.ParseCatalog` + `Validate`.)

**Already-modified `auto-rag-guard.sh`?** A project running `vaultmind hooks install --force` will overwrite a hand-edited copy wholesale (workhorse hit this exact case post-absorption). Before `--force`:
1. Back up the file: `cp .claude/scripts/auto-rag-guard.sh .claude/scripts/auto-rag-guard.sh.local-backup`
2. Read the diff between the canonical (`vaultmind hooks install --json` to a tempdir, or read `internal/hookscripts/auto-rag-guard.sh` from a vaultmind clone) and your local copy.
3. Migrate local customizations into env-var overrides (above) or the wrapper-script pattern. Anything that can't be expressed as env vars usually means the canonical engine needs a new env-var knob — file an issue rather than forking the script.
4. Run `vaultmind hooks install --force`. Your wrapper / settings.json env vars stay; the canonical engine is freshly canonical.
5. `vaultmind doctor` should report zero hook drift.

This is also how `vaultmind hooks install --force` stays SSOT-clean: the canonical engine bytes don't change per-consumer; only the invocation context does.

If the user is unsure whether auto-RAG fits their workflow, skip §6e for the first session. They can opt in later by adding the settings.json entry — `vaultmind hooks install` already wrote the scripts.

---

## 7. Diff-before-write protocol

This applies to every file vaultmind onboarding might mutate that wasn't created by vaultmind:

- `.claude/settings.json` — merge entries; never overwrite.
- `AGENTS.md` / `CLAUDE.md` — append a clearly-marked section if relevant; never replace.
- Any existing `.md` file in a migration — additive frontmatter only; never touch body.

The protocol:

1. Read the file.
2. Compute the proposed change.
3. **Show the user a unified diff.**
4. Wait for affirmative.
5. Write.
6. Confirm written.

Never batch-write across files without showing each diff. The user's trust is the load-bearing thing being established in this onboarding — a single covert mutation breaks it irrecoverably.

---

## 8. Verification checklist

When you think you're done:

- [ ] `vaultmind doctor --vault "<path>"` is green (or the `missing_required_field` count is zero on indexed notes).
- [ ] `vaultmind ask "<sample query>" --vault "<path>"` returns a sensible top hit.
- [ ] `vaultmind self --vault "<path>"` shows recent access events (the queries you just ran).
- [ ] If Claude Code wired: `.claude/settings.json` has the hook entries; `.claude/scripts/` has the four scripts; restarting Claude Code surfaces an `IDENTITY CONTEXT` system-reminder.
- [ ] User has confirmed each diff and is unsurprised by what's now on disk.

Tell the user explicitly what was changed and where to find sidecar logs (`~/.vaultmind/userprompt-hook/`, `~/.vaultmind/preread-track/`, `~/.vaultmind/persona-eval/`).

---

## 9. Failure-mode appendix

**ORT install fails (libonnxruntime missing).** Tell the user how to install: `brew install onnxruntime` on macOS, then `task setup:ort` and re-build. Until that's done, vaultmind operates on the pure-Go path with MiniLM-only embeddings — usable but degraded retrieval quality. Migration / scaffolding still works.

**`.claude/settings.json` is malformed.** Don't try to fix it. Tell the user: *"Your settings.json doesn't parse as JSON. I'll point at the syntax error and you decide whether to fix it before continuing." If they fix it, re-run §6.

**Existing hook script with our name.** Refuse. Offer to write to a side path (`vault-track-read.v2.sh`) and let the user manually merge later.

**The user's repo has 200+ markdown files in random structure.** Offer to start with one directory only. *"I see [N] files across [M] directories. Indexing all of them at once is risky if some aren't really knowledge content. Pick one directory to start with — we can add others later."*

**Migration would touch a file under `.git/` or `node_modules/`.** Refuse. The exclude list should catch this; if it doesn't, your survey logic is wrong.

**The user is using a non-Claude-Code agent.** Today's hook templates are Claude-Code-specific. Skip §6 and tell the user: *"Your agent (Cursor, Codex, etc.) needs different integration. The CLI works fine; the auto-loading-of-persona-on-session-start needs to be configured in your agent's hook system. See [vaultmind repo's load-persona.sh] as a reference for the pattern."*

**The user wants to undo.** Vaultmind doesn't have an `uninstall` command yet. Tell them what to delete: `<vault-path>` (entirely), the four `.claude/scripts/*` files you wrote, the matching entries in `.claude/settings.json`. Suggest they make a git commit *before* you start writing anything (gives them a clean revert point).

---

## 10. After onboarding

Tell the user:

- The vault is theirs to curate. Edit any markdown file by hand — vaultmind picks up changes on the next `vaultmind index`.
- The agent (you) will retrieve from the vault automatically once hooks are wired. No further setup needed per session.
- For day-to-day use as an agent, see `docs/AGENT_USAGE.md`.
- For the architectural design, the plasticity roadmap is at `vaultmind-identity/references/plasticity-priority-order.md` (or its in-vault equivalent in the user's vault if they migrated docs).

---

## Known issues for v2 (surfaced by 2026-05-04 dogfood)

These were caught walking this doc against a real local repo (greenfield via `/tmp/dogfood-onboarding`, migration shape via `daana-content-machine`). They're real but not push-blocking; named here so the next iteration knows what to fix.

- **`vaultmind ask` returns `_path:README.md` hits in top-2.** The vault's own README hits its own retrieval. Fix candidate: exclude README.md from indexing by default, or filter in the retriever's domain-note branch.
- **`related: [/path/to/file.md]` (Obsidian-style path links)** in migrated files don't auto-resolve to vaultmind ids. Vaultmind tolerates it (passes through unchanged), but cross-note retrieval doesn't follow these links. v2 candidate: alias `related` → `related_ids` AND a path-to-id resolver pass during indexing.
- **Debug log on stdout** during retrieval (`INF Using config file...`). Annoying for the agent's grep/parse. Fix candidate: log to stderr only by default; stdout reserved for command output.
- **Vaultmind's own `Next steps` output uses bare `vaultmind`** (PATH-assumption — same root cause as §1a's first-version bug). Fix candidate: vaultmind detects how it was invoked and uses that in its hint output.
- **No URL fallback for this doc.** The entry sentence assumes the user has a local clone. If they `go install` (future) or use a release binary, the local path doesn't exist. Fix candidate once the repo is public: the entry sentence cites both a local path AND a GitHub URL, agent uses whichever the user has.
- **Per-vault adapter for non-Claude-Code agents** (Cursor, Codex, Aider, etc.) — §6 + §9 acknowledge this is out of scope; v2+ deserves a dedicated doc per agent.

## Source

- Plan note: `vaultmind-identity/references/onboarding-ax-design.md` — the full design rationale, lens-walked decisions, probe data on shahname-rts and content-machine.
- Companion arc: `arc-the-lighter-move-is-the-work` — the discipline this doc honors. v1 covers greenfield + migration; hybrid and adapter-for-other-agents are deferred until reality demands.
- First real user: Siavoush. This doc is the script he'll be onboarded with. Capture what breaks; it informs v2.
- Dogfood pass: 2026-05-04, surfaced 5 critical/important fixes (PATH assumption, `.claude/` exclude, empty type vocabulary, config-as-markdown filter, hook path-templating) and 5 nice-to-haves (above).
