# Associative Memory Model

> See also: [data model](03-data-model.md), [response shapes](09-response-shapes.md), [storage model](12-storage-model.md)

## Edge Types

| Edge type | Origin | Confidence | Source component |
|-----------|--------|------------|-----------------|
| `explicit_link` | Body wikilink or markdown link | `high` | Parser |
| `explicit_embed` | Obsidian embed (`![[...]]`) | `high` | Parser |
| `explicit_relation` | Stable-ID frontmatter field (`related_ids`, `parent_id`, `source_ids`) | `high` | Frontmatter |
| `alias_mention` | Unlinked body text matching a known alias | `medium` | Indexer |
| `tag_overlap` | Shared tags between notes (weighted by specificity) | `low` | Indexer |
| `dataview_reference` | Dataview query referencing tag/path/note | `low` | Parser |

### Confidence vs Provenance

The `confidence` field indicates **edge provenance tier** — how the edge was derived — not epistemic certainty about the relationship. `high` means the author explicitly created the link; `low` means the system inferred it from structural overlap. All edges within a tier are equally reliable; the tiers distinguish authorial intent from algorithmic inference.

Future versions may add a companion `edge_source` field with finer-grained provenance. The `confidence` field name is retained for backward compatibility.

## Link Extraction

The parser extracts outbound edges from:

- Wikilinks: `[[Note]]`
- Aliased wikilinks: `[[Note|Display]]`
- Heading links: `[[Note#Heading]]`
- Block links: `[[Note#^block]]`
- Embeds: `![[Note]]`
- Markdown links to vault-relative resources
- Stable-ID frontmatter fields: `parent_id`, `related_ids`, `source_ids`

Inbound links are **always computed** by inverting resolved outbound edges. Never stored as canonical metadata.

Unresolved links are recorded with the raw reference text for diagnostics and repair.

## Alias Mention Detection

When indexing, the indexer scans note body text for unlinked mentions of known aliases and titles.

**Matching rules:**
- Match on word boundaries only (no partial-word matches)
- Case-insensitive comparison
- Minimum alias length: 3 characters (to avoid false positives on short words)
- Skip matches inside code fences, YAML frontmatter, and existing wikilinks
- Each unique alias match produces one `alias_mention` edge per source-target pair (not per occurrence)

## Tag Overlap Specificity

Tag overlap edges are weighted by inverse document frequency:

```
specificity(tag) = log(total_domain_notes / notes_with_tag)
overlap_score(A, B) = sum(specificity(tag) for tag in shared_tags(A, B))
```

Tag overlap edges are only created when `overlap_score >= 1.0`. The score is stored as the edge weight.

## Memory Recall

Given a note, VaultMind answers:

- **What is this note?** — identity, type, status, frontmatter
- **What does it point to?** — outbound links by edge type
- **What points to it?** — inbound links by edge type
- **What is nearby?** — neighborhood traversal to configurable depth
- **What names refer to it?** — aliases
- **What is likely relevant but not hard-linked?** — inferred associations
- **What evidence supports each relation?** — edge origin and confidence

### `memory recall` vs `links neighbors`

- `links neighbors` returns raw graph edges with minimal metadata — useful for graph exploration
- `memory recall` returns enriched nodes with full frontmatter — useful for agent context building

Both support `--depth`, `--min-confidence`, and `--max-nodes` (default: 200). The difference is output richness, not traversal logic.

### Traversal Algorithm

All depth>0 traversals use **breadth-first search with a visited set** keyed on `note_id`. This prevents infinite loops on cyclic graphs (which are common — `related_ids` and wikilinks can form cycles). BFS is required because node `distance` tracking depends on it.

The `--max-nodes` cap (default 200) limits total nodes returned, preventing fan-out explosion at hub notes.

## Context Packs

The `context-pack` command assembles a bounded retrieval payload for agent consumption.

**Budget unit:** estimated tokens (1 token ~ 4 characters). `--budget` accepts integer token count. Default: `4096`.

**Packing algorithm:**

1. Include target note's full frontmatter and body. If this alone exceeds budget, truncate body to fit and set `truncated: true`.
2. Include frontmatter-only summaries of `explicit_relation` neighbors, sorted by `updated` desc.
3. Include frontmatter-only summaries of `explicit_link` neighbors (outbound then inbound), sorted by `updated` desc.
4. If budget remains, include frontmatter-only summaries of `medium`-confidence neighbors.
5. Stop when budget exhausted. Set `budget_exhausted: true` if items omitted.

Each included note carries its edge type and confidence relative to the target.
