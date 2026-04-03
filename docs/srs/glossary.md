# Glossary

Quick-reference definitions. See linked sections for full details.

| Term | Definition | Details |
|------|-----------|---------|
| **Domain note** | `.md` file with both `id` and `type` in frontmatter. Fully indexed. | [03](03-data-model.md) |
| **Unstructured note** | `.md` file lacking `id`, `type`, or both. Partially indexed (FTS, links). | [03](03-data-model.md) |
| **Canonical** | Data that is authoritative and human-authored. Markdown files, frontmatter, filesystem, Git. | [03](03-data-model.md) |
| **Derived** | Data rebuilt from canonical sources. SQLite index, backlinks, inferred edges, FTS. | [03](03-data-model.md) |
| **Entity resolution** | Process of resolving a reference to a note via id > title > alias > normalized tiers. | [03](03-data-model.md) |
| **Stable ID** | The immutable `id` frontmatter field. Primary identity mechanism. | [03](03-data-model.md) |
| **Edge type** | Classification of a relationship between notes (e.g., `explicit_link`, `alias_mention`). | [05](05-memory-model.md) |
| **Confidence** | `high`, `medium`, or `low` — how certain we are about an edge. | [05](05-memory-model.md) |
| **Context pack** | Token-budgeted retrieval payload assembled for agent consumption. | [05](05-memory-model.md) |
| **Generated region** | Markdown section between `VAULTMIND:GENERATED` markers. Agent-writable. | [06](06-mutation-model.md) |
| **Type registry** | Config-defined vocabulary of note types with required/optional fields. | [18](18-config-spec.md) |
| **Plan file** | JSON document describing a batch of atomic mutations. | [10](10-plan-files.md) |
| **Git policy matrix** | Rules governing which operations are allowed based on repo state. | [07](07-git-model.md) |
| **JSON envelope** | Standard wrapper for all `--json` output: `{command, status, result, meta}`. | [08](08-agent-contract.md) |
| **Frontmatter normalize** | Reformat frontmatter: key order, list conversion, date format, snake_case. | [06](06-mutation-model.md) |
| **Alias mention** | Medium-confidence edge from unlinked body text matching a known alias. | [05](05-memory-model.md) |
| **Tag overlap** | Low-confidence edge from shared tags, weighted by inverse document frequency. | [05](05-memory-model.md) |
