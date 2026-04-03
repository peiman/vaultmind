# VaultMind SRS v3

Associative memory system for AI agents, built on a Git-backed, Obsidian-compatible Markdown vault. Implemented as a Go CLI (from `github.com/peiman/ckeletin-go`) with an optional API server.

**The vault is the single source of truth. Git is the system of record. VaultMind is the memory layer.**

## Navigation

| File | Contents | When to read |
|------|----------|-------------|
| [01-overview](01-overview.md) | Principles, scope, users | Starting out, understanding constraints |
| [02-architecture](02-architecture.md) | Layers, components, Go packages | Planning new packages or components |
| [03-data-model](03-data-model.md) | Canonical vs derived, note types, identity, entity resolution | Parser/indexer work, understanding note lifecycle |
| [04-frontmatter-schema](04-frontmatter-schema.md) | Field tiers, type registry, anti-patterns | Frontmatter parsing, validation, mutation |
| [05-memory-model](05-memory-model.md) | Edge types, link extraction, recall, context packs | Graph/memory engine work |
| [06-mutation-model](06-mutation-model.md) | Allowed/forbidden surfaces, workflow, refusals, dataview regions | Any write operation |
| [07-git-model](07-git-model.md) | Policy matrix, commits, boundaries | Git integration layer |
| [08-agent-contract](08-agent-contract.md) | JSON envelope, stable output policy | CLI output formatting |
| [09-response-shapes](09-response-shapes.md) | All JSON response shapes | Implementing any command's output |
| [10-plan-files](10-plan-files.md) | Plan format, operations, execution semantics | `apply` command |
| [11-cli-reference](11-cli-reference.md) | Full command listing with flags | Implementing any CLI command |
| [12-storage-model](12-storage-model.md) | SQLite schema, FTS, indexing strategy | Index/query layer |
| [13-validation-rules](13-validation-rules.md) | All validation rules with severity | `frontmatter validate`, pre-mutation checks |
| [14-safety-model](14-safety-model.md) | Safety guarantees, concurrency model | Write paths, conflict handling |
| [15-nonfunctional](15-nonfunctional.md) | Perf targets, portability, determinism, compatibility | Performance work, CI setup |
| [16-risks](16-risks.md) | Risks and mitigations | Architecture decisions |
| [17-build-phases](17-build-phases.md) | Phased plan, acceptance criteria | Sprint planning, milestone tracking |
| [18-config-spec](18-config-spec.md) | Config file location, schema, discovery | Config loading, type registry |
| [19-template-spec](19-template-spec.md) | Template system for `note create` | Template engine |
| [glossary](glossary.md) | Term definitions | Quick lookup |
| [decisions](decisions.md) | Design decisions, v2 issues addressed | Understanding rationale |

## Quick Reference

- **Core identity fields:** `id` (immutable, unique) + `type` (controlled vocabulary) — see [03](03-data-model.md)
- **Entity resolution order:** id > title > alias > normalized > unresolved — see [03](03-data-model.md)
- **Edge confidence levels:** `high`, `medium`, `low` — see [05](05-memory-model.md)
- **Mutation workflow:** read+hash > compute > diff > validate > verify-hash > atomic-write > stage+commit — see [06](06-mutation-model.md)
- **Git policy:** refuse writes on dirty target files, merge/rebase in progress — see [07](07-git-model.md)
- **JSON envelope:** `{command, status, warnings, errors, result, meta}` — see [08](08-agent-contract.md)
- **Config location:** `.vaultmind/config.yaml` in vault root — see [18](18-config-spec.md)

## Version History

- **v3** (2026-04-03): Chunked for agent consumption. Filled gaps from v2 review (config spec, template spec, search pagination, FTS schema fix, normalize definition, alias matching rules, tag specificity formula). See [decisions](decisions.md).
- **v2** (2026-04-03): Monolithic SRS. Source: `/Users/peiman/Downloads/vaultmind-srs-v2.md`
