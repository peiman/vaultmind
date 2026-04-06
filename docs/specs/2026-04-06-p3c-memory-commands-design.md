# P3c: Memory Commands — Design Spec

> Phase 3 sub-project C. Associative memory queries: recall, related, context-pack.
>
> SRS references: [05-memory-model.md](../srs/05-memory-model.md), [09-response-shapes.md](../srs/09-response-shapes.md), [11-cli-reference.md](../srs/11-cli-reference.md)

## Goal

Implement `memory recall`, `memory related`, and `memory context-pack` commands in a new `internal/memory/` package. These are the core agent-facing memory retrieval primitives.

## Scope

**In scope:**
- `internal/memory/` package with `Recall`, `Related`, `ContextPack` functions
- Three CLI commands: `memory recall`, `memory related`, `memory context-pack`
- JSON responses matching SRS shapes
- Token estimation via character counting (upgradeable)

**Out of scope:**
- Note creation (P3d)
- Graph traversal engine (P3b — already done, reused here)

## `memory recall`

### Function

```go
func Recall(resolver *graph.Resolver, db *index.DB, cfg RecallConfig) (*RecallResult, error)

type RecallConfig struct {
    Input         string
    Depth         int    // default 2
    MinConfidence string // default "low"
    MaxNodes      int    // default 200
}
```

### Algorithm

1. Resolve input → start ID via `resolver.Resolve(input)`
2. Call `resolver.Traverse(TraverseConfig{...})` — reuses P3b BFS engine
3. For each node in traversal result, query full frontmatter from `notes` + `frontmatter_kv` tables
4. Build enriched result with nodes (including frontmatter) and a separate edges array

### Response Shape

```json
{
  "target_id": "proj-payment-retries",
  "depth": 2,
  "max_nodes": 200,
  "max_nodes_reached": false,
  "nodes": [
    {
      "id": "proj-payment-retries",
      "type": "project",
      "title": "Payment Retries",
      "distance": 0,
      "frontmatter": {"status": "active", "tags": ["billing"]}
    },
    {
      "id": "concept-idempotency",
      "type": "concept",
      "title": "Idempotency",
      "distance": 1,
      "edge_from_parent": {"edge_type": "explicit_relation", "confidence": "high"},
      "frontmatter": {"tags": ["patterns"]}
    }
  ],
  "edges": [
    {"source_id": "proj-payment-retries", "target_id": "concept-idempotency", "edge_type": "explicit_relation", "confidence": "high"}
  ]
}
```

### Difference from `links neighbors`

`links neighbors` returns raw graph edges with minimal metadata (node IDs + distances). `memory recall` returns enriched nodes with full frontmatter and a structured edges array. Both use the same BFS traversal under the hood.

### CLI

```
vaultmind memory recall <id-or-path> [--depth N] [--min-confidence low|medium|high] [--max-nodes N] [--json]
```

Defaults: `--depth 2`, `--min-confidence low`, `--max-nodes 200`.

## `memory related`

### Function

```go
func Related(resolver *graph.Resolver, db *index.DB, cfg RelatedConfig) (*RelatedResult, error)

type RelatedConfig struct {
    Input string
    Mode  string // "explicit", "inferred", "mixed"
}
```

### Algorithm

1. Resolve input → target ID
2. Query direct edges from links table (depth 1 only, no BFS needed)
3. Filter by mode:
   - `explicit`: confidence = "high" only (explicit_link, explicit_embed, explicit_relation)
   - `inferred`: confidence = "medium" or "low" only (alias_mention, tag_overlap)
   - `mixed`: all edges
4. For each related note, query basic metadata (type, title) + edge info
5. For `tag_overlap` edges, include `weight` as `score` (shared_tags omitted in v1 for simplicity)

### Response Shape

```json
{
  "target_id": "proj-payment-retries",
  "mode": "mixed",
  "related": [
    {
      "id": "concept-idempotency",
      "type": "concept",
      "title": "Idempotency",
      "edge_type": "explicit_relation",
      "confidence": "high",
      "origin": "frontmatter.related_ids"
    },
    {
      "id": "proj-email-retries",
      "type": "project",
      "title": "Email Retries",
      "edge_type": "tag_overlap",
      "confidence": "low",
      "score": 2.4
    }
  ]
}
```

### CLI

```
vaultmind memory related <id-or-path> [--mode explicit|inferred|mixed] [--json]
```

Default: `--mode mixed`.

## `memory context-pack`

### Function

```go
func ContextPack(resolver *graph.Resolver, db *index.DB, cfg ContextPackConfig) (*ContextPackResult, error)

type ContextPackConfig struct {
    Input  string
    Budget int // token budget, default 4096
}
```

### Token Estimation

```go
// EstimateTokens estimates token count from content using character-based heuristic.
// Uses 1 token ≈ 4 characters as per SRS.
// NOTE: This can be upgraded to a proper tokenizer (e.g., tiktoken) in future versions
// without changing the interface.
func EstimateTokens(content string) int {
    return (len(content) + 3) / 4 // ceiling division
}
```

### Packing Algorithm (from SRS)

1. Include target note's full frontmatter and body. If this alone exceeds budget, truncate body to fit and set `truncated: true`.
2. Include frontmatter-only summaries of `explicit_relation` neighbors, sorted by `updated` desc.
3. Include frontmatter-only summaries of `explicit_link` neighbors (outbound then inbound), sorted by `updated` desc.
4. If budget remains, include frontmatter-only summaries of `medium`-confidence neighbors.
5. Stop when budget exhausted. Set `budget_exhausted: true` if items omitted.

Each included context note carries its edge type and confidence relative to the target.

### Response Shape

```json
{
  "target_id": "proj-payment-retries",
  "budget_tokens": 4096,
  "used_tokens": 3842,
  "budget_exhausted": false,
  "truncated": false,
  "target": {
    "id": "proj-payment-retries",
    "frontmatter": {"status": "active", "tags": ["billing"]},
    "body": "Full body text..."
  },
  "context": [
    {
      "id": "concept-idempotency",
      "edge_type": "explicit_relation",
      "confidence": "high",
      "frontmatter": {"title": "Idempotency"},
      "body_included": false
    }
  ]
}
```

### CLI

```
vaultmind memory context-pack <id-or-path> [--budget N] [--json]
```

Default: `--budget 4096`.

## Frontmatter Enrichment

All three commands need to load frontmatter for notes by ID. Shared helper:

```go
// LoadNoteMeta loads frontmatter metadata for a note by ID.
func LoadNoteMeta(db *index.DB, noteID string) (*NoteMeta, error)

type NoteMeta struct {
    ID          string                 `json:"id"`
    Type        string                 `json:"type"`
    Title       string                 `json:"title"`
    Status      string                 `json:"status,omitempty"`
    Created     string                 `json:"created,omitempty"`
    Updated     string                 `json:"updated,omitempty"`
    Frontmatter map[string]interface{} `json:"frontmatter"`
}

// LoadNoteBody loads body text for a note by ID.
func LoadNoteBody(db *index.DB, noteID string) (string, error)
```

These query the `notes` table for core fields and `frontmatter_kv` for extra fields.

## Testing Strategy

### Recall tests
- Basic recall at depth 1: verify enriched nodes have frontmatter
- Depth 2: verify 2-hop nodes present
- MaxNodes cap: verify result capped
- Edges array: verify source/target/type for each edge

### Related tests
- Explicit mode: only high-confidence edges returned
- Inferred mode: only medium/low edges
- Mixed mode: all edges
- No edges: empty related list

### ContextPack tests
- Budget sufficient: target + context notes fit
- Budget exceeded: `budget_exhausted: true`, items omitted
- Body truncation: very small budget, body truncated
- EstimateTokens: verify `len/4` math
- Packing priority: explicit_relation before explicit_link before medium

### Coverage target
85%+ for `internal/memory/` package.

## Design Decisions

### DD-1: New `internal/memory/` package

**Choice:** Recall/Related/ContextPack in `internal/memory/`, separate from graph and query.

**Rationale:** These are enrichment functions that combine graph traversal with data loading. They import `graph` (for traversal) and `index` (for DB queries). The `query` package stays for simpler single-table queries; `memory` handles the multi-step enrichment logic.

### DD-2: Character-based token estimation (upgradeable)

**Choice:** `len(content) / 4` per SRS specification.

**Rationale:** Simple, no dependency. Accurate enough for budgeting. The `EstimateTokens` function is isolated — swapping to tiktoken or similar in a future version requires changing only this one function, no interface changes.

### DD-3: Recall reuses Traverse from P3b

**Choice:** `Recall` calls `resolver.Traverse()` then enriches each node.

**Rationale:** No duplicate BFS code. The traversal engine is tested and correct. Recall adds the enrichment layer on top.

## File Inventory

| File | Purpose |
|------|---------|
| `internal/memory/common.go` | `LoadNoteMeta`, `LoadNoteBody`, `NoteMeta`, `EstimateTokens` |
| `internal/memory/recall.go` | `Recall`, `RecallConfig`, `RecallResult` |
| `internal/memory/related.go` | `Related`, `RelatedConfig`, `RelatedResult` |
| `internal/memory/contextpack.go` | `ContextPack`, `ContextPackConfig`, `ContextPackResult` |
| `internal/memory/common_test.go` | Shared helper tests |
| `internal/memory/recall_test.go` | Recall tests |
| `internal/memory/related_test.go` | Related tests |
| `internal/memory/contextpack_test.go` | ContextPack tests |
| `cmd/memory.go` | Parent `memory` command |
| `cmd/memory_recall.go` | `memory recall` subcommand |
| `cmd/memory_related.go` | `memory related` subcommand |
| `cmd/memory_context_pack.go` | `memory context-pack` subcommand |
| `internal/config/commands/memory_config.go` | Config registration for all three commands |
