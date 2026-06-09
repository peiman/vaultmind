# VaultMind Commands

Every user-facing command, grouped by intent, with its when-to-use trigger.
Generated from the command tree — do not edit by hand (run `task generate:docs:commands`).

## Retrieval & memory:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind ask` | Compound search + context-pack: answer 'what do I know about X?' | you want to answer "what do I know about X?" — search plus packed context in one step. |
| `vaultmind memory` | Traverse the note graph and assemble context for agents | you need the low-level graph primitives behind ask: links, neighbors, related, pack, summarize. |
| `vaultmind memory links` | List a note's directed wikilink edges (outbound, inbound, or both) | you want a note's directed wikilink edges — outbound, inbound, or both. |
| `vaultmind memory neighbors` | Traverse the graph from a note (BFS) and return enriched neighbors | you want the enriched graph around a note via depth-limited BFS, with full frontmatter. |
| `vaultmind memory pack` | Pack a note plus ranked context within a token budget | you want a token-budgeted context payload ready to ship to an agent. |
| `vaultmind memory related` | List notes related to a target, filtered by edge type | you want a simple ranked list of directly connected notes, filtered by edge type. |
| `vaultmind memory summarize` | Assemble material from specific note IDs for agent synthesis | you have a known list of note IDs and want their material assembled for synthesis. |
| `vaultmind note` | Read, create, and batch-fetch notes by ID | you want to read, create, or batch-fetch a specific note by ID. |
| `vaultmind note create` | Create a note from a template with field overrides | you want to create a new note from its type template with field overrides. |
| `vaultmind note get` | Get one note's full content and metadata by ID | you know a note's ID or path and want its full content (with access tracking). |
| `vaultmind note mget` | Fetch multiple notes by ID in one call | you have several note IDs and want them all in one batched fetch. |
| `vaultmind resolve` | Resolve a fragment, alias, title, or path to canonical note IDs | you have a fragment, alias, title, or path and need the canonical note ID. |
| `vaultmind search` | Search vault notes by keyword, semantic similarity, or both | you want a ranked list of hits to browse and pick from, without packed context. |
| `vaultmind self` | Show your memory state — recent, hot, and stale notes | you want to see your own memory state — recent, hot, and stale notes. |

## Vault maintenance:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind apply` | Execute an AI-generated plan to mutate vault notes | you have an AI-generated JSON plan and want to execute its note mutations. |
| `vaultmind dataview` | Manage template-generated regions in vault notes | you manage template-generated regions in notes and need to render or lint their markers. |
| `vaultmind dataview lint` | Scan the vault for broken or duplicate generated-region markers | you want to catch malformed or duplicated VAULTMIND:GENERATED markers before rendering. |
| `vaultmind dataview render` | Render a note's generated region from its template | you edited a template and want to refresh a note's generated region. |
| `vaultmind doctor` | Vault health hub: diagnose a vault and report issues | you want a read-only health overview of a vault (or every vault, with --all). |
| `vaultmind doctor heal` | Apply all auto-fixable repairs doctor found | doctor found auto-fixable issues and you want to apply every repair at once. |
| `vaultmind doctor heal wikilinks` | Rewrite Obsidian-incompatible wikilinks to [[filename\|Title]] | doctor flagged Obsidian-incompatible wikilinks and you want them rewritten to [[filename\|Title]]. |
| `vaultmind frontmatter` | Inspect and mutate YAML frontmatter across vault notes | you need to audit, validate, or programmatically edit YAML frontmatter across notes. |
| `vaultmind frontmatter fix` | Backfill missing "created" frontmatter on domain notes | you are migrating notes and need to backfill the missing "created" field. |
| `vaultmind frontmatter merge` | Merge multiple frontmatter fields from a YAML file into a note | you want to merge many key/value pairs from a YAML file into one note at once. |
| `vaultmind frontmatter normalize` | Normalize one note's frontmatter (keys, aliases, dates, snake_case) | you want to clean up one note's frontmatter formatting — key order, aliases, dates, snake_case. |
| `vaultmind frontmatter set` | Set one frontmatter field on a note | you want to set a single frontmatter field on one note, schema-validated. |
| `vaultmind frontmatter unset` | Remove one frontmatter field from a note | you want to remove one frontmatter field from a note. |
| `vaultmind frontmatter validate` | Check vault notes for frontmatter rule violations | you want to catch missing fields, bad statuses, unknown types, or broken refs before indexing. |
| `vaultmind index` | Scan and index vault notes into SQLite, optionally embedding | vault notes changed and you need to refresh the SQLite index (and optionally embeddings). |
| `vaultmind schema` | Query the vault's type schema | you need to discover the vault's note types, required fields, and valid statuses. |
| `vaultmind schema list-types` | List every note type with its required fields and valid statuses | you want every registered type with its required fields and valid statuses before creating notes. |

## Identity & sessions:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind arc` | Surface arc-distillation candidates from episodes (propose-only) | you want to surface arc-distillation candidate moments from episodes (propose-only). |
| `vaultmind arc candidates` | Surface candidate transformation moments for arc distillation | you finished a session and want candidate transformation moments to review for arcs. |
| `vaultmind episode` | Capture Claude Code sessions as episodic-memory artifacts | you want to capture Claude Code sessions as episodic-memory artifacts. |
| `vaultmind episode capture` | Convert session transcripts into episode notes | you have a session transcript (or a directory of them) to convert into episode notes. |
| `vaultmind hooks` | Manage VaultMind's Claude Code hook scripts | you need to install, remove, or check VaultMind's Claude Code hook scripts. |
| `vaultmind hooks install` | Install Claude Code hook scripts into a project | you want to wire VaultMind into a project by writing its hook scripts. |
| `vaultmind hooks uninstall` | Remove VaultMind's Claude Code hook entries from a project | you want to remove VaultMind's Claude Code hook entries from a project. |
| `vaultmind identity` | Contract-B agent identity: keypair custody and signing | you need Contract-B agent identity: mint a keypair or sign an entry via the keyless signer. |
| `vaultmind identity init` | Mint an agent keypair and seal the private key to the signer | you are setting up an agent and need to mint its ed25519 keypair and seal the private key to the signer. |
| `vaultmind identity sign` | Validate, canonicalize, and sign an entry via the keyless signer | you have a Contract-B entry to sign and want it validated, canonicalized, and signed by the keyless signer. |
| `vaultmind identity sign-envelope` | Sign a chat message envelope via the keyless signer (Contract-B slice 5) | you have a chat MESSAGE envelope to sign so a receiving daemon can verify the signature and the signer's registry binding. |
| `vaultmind identity sign-registry` | Sign a trust-root registry via the keyless signer (Contract-B) | you have a trust-root registry to sign so consumers can verify the root signature, anti-rollback epoch, and freshness at load. |
| `vaultmind init` | Scaffold a fresh persona-shaped vault, ready for you and your agent | you are starting fresh and need to scaffold a new persona-shaped vault. |

## Setup & introspection:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind completion` | Generate the shell autocompletion script | you want to install shell tab-completion for the vaultmind command. |
| `vaultmind config` | Manage and validate application configuration | you want to manage or validate the application's configuration file. |
| `vaultmind config validate` | Validate a configuration file | you want to check a config file for correctness, security, and unknown keys. |
| `vaultmind docs` | Generate documentation | you want to generate documentation about the app and its configuration. |
| `vaultmind docs commands` | Generate the grouped command reference (COMMANDS.md) | you want a generated grouped reference of every command, each with its when-to-use. |
| `vaultmind docs config` | Generate the configuration-options reference | you want a generated reference of every configuration option. |
| `vaultmind experiment` | Experiment tracking and reporting | you want to inspect experiment tracking: retrieval quality, usage, traces, comparisons. |
| `vaultmind experiment compare` | Surface where retrieval variants disagree, no labels needed | you want to see where retrieval variants disagree, without labeled ground truth. |
| `vaultmind experiment report` | Measure retrieval quality: Hit@K and MRR per variant | you want to measure retrieval quality — Hit@K and MRR per variant. |
| `vaultmind experiment summary` | Memory usage overview: top recalled notes, session gaps | you want a memory-usage overview — top recalled notes and session-gap stats. |
| `vaultmind experiment trace` | Drill into a session's or note's retrieval history | you want to drill into one session's or note's retrieval history. |
| `vaultmind export` | Export experiment data as a sanitized JSONL snapshot | you want a sanitized JSONL snapshot of experiment data to share with the VaultMind team. |
| `vaultmind git` | Inspect git repository state relevant to vault operations | you want git repository state relevant to VaultMind mutation policies. |
| `vaultmind git status` | Report git branch, dirty, and merge/rebase state for a vault | a script or agent needs to gate on the vault's branch, dirty, or merge/rebase state. |
| `vaultmind ping` | Respond with a pong (connectivity smoke test) | you want to smoke-test that the binary runs and renders output. |
| `vaultmind version` | Print the version, commit, and build date | you want the build version, commit, and date. |
