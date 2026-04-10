# P3b: Graph Traversal — Design Spec

> Phase 3 sub-project B. BFS graph traversal and `links neighbors` command.
>
> SRS references: [05-memory-model.md](../srs/05-memory-model.md), [09-response-shapes.md](../srs/09-response-shapes.md), [11-cli-reference.md](../srs/11-cli-reference.md)

## Goal

Implement BFS graph traversal with visited set, depth tracking, confidence filtering, and max-nodes cap. Expose as `links neighbors` command. This traversal logic is reused by `memory recall` in P3c.

## Scope

**In scope:**
- `internal/graph/traverse.go` — BFS traversal engine
- `links neighbors` CLI command with `--depth`, `--min-confidence`, `--max-nodes` flags
- `internal/query/neighbors.go` — query function formatting traversal results
- JSON response matching SRS shape
- Bidirectional traversal (outbound + inbound edges)

**Out of scope:**
- `memory recall` enrichment (P3c — reuses traversal)
- `memory related`, `memory context-pack` (P3c)

## BFS Traversal

### Types

```go
// TraverseConfig holds parameters for graph traversal.
type TraverseConfig struct {
    StartID       string
    MaxDepth      int    // default 1
    MinConfidence string // "low", "medium", "high"
    MaxNodes      int    // default 200
}

// TraverseNode is one node in the traversal result.
type TraverseNode struct {
    ID       string
    Distance int
    EdgeFrom *TraverseEdge // edge that discovered this node (nil for start node)
}

// TraverseEdge describes the edge that connected a node to its parent in the BFS.
type TraverseEdge struct {
    SourceID   string
    EdgeType   string
    Confidence string
    Weight     float64
}

// TraverseResult is the output of a BFS traversal.
type TraverseResult struct {
    StartID         string
    Nodes           []TraverseNode
    MaxNodesReached bool
}
```

### Function

```go
// Traverse performs BFS from a start node with configurable depth,
// confidence filter, and node cap.
func (r *Resolver) Traverse(db *index.DB, cfg TraverseConfig) (*TraverseResult, error)
```

Note: The `Traverse` method lives on the existing `Resolver` struct in `internal/graph/` since it already holds a `*index.DB` reference. Alternatively it can accept `db` as a parameter — the implementer should choose based on what fits the existing Resolver pattern.

### Algorithm

1. Initialize FIFO queue with `(StartID, distance=0)`
2. Initialize visited set `map[string]bool`
3. While queue is non-empty and `len(result.Nodes) < MaxNodes`:
   a. Dequeue `(nodeID, distance)`
   b. If already visited, skip
   c. Mark visited, append `TraverseNode{ID: nodeID, Distance: distance}` to result
   d. If `distance < MaxDepth`:
      - Query outbound edges: `SELECT dst_note_id, edge_type, confidence, weight FROM links WHERE src_note_id = ? AND resolved = TRUE`
      - Query inbound edges: `SELECT src_note_id, edge_type, confidence, weight FROM links WHERE dst_note_id = ? AND resolved = TRUE`
      - Filter edges by MinConfidence
      - For each neighbor not in visited: enqueue at `distance + 1`
4. If `len(result.Nodes) >= MaxNodes`, set `MaxNodesReached = true`

### Confidence Filtering

Confidence levels are ordered: `low < medium < high`.

`--min-confidence medium` excludes `low`-confidence edges (`tag_overlap`).
`--min-confidence high` excludes both `low` and `medium` (`tag_overlap` + `alias_mention`).

```go
var confidenceLevel = map[string]int{"low": 0, "medium": 1, "high": 2}

func meetsConfidence(edgeConfidence, minConfidence string) bool {
    return confidenceLevel[edgeConfidence] >= confidenceLevel[minConfidence]
}
```

### Edge Queries

Both outbound and inbound edges are followed — the graph is treated as undirected for neighborhood exploration.

```sql
-- Outbound
SELECT dst_note_id, edge_type, confidence, weight
FROM links WHERE src_note_id = ? AND resolved = TRUE AND dst_note_id IS NOT NULL

-- Inbound
SELECT src_note_id, edge_type, confidence, weight
FROM links WHERE dst_note_id = ? AND resolved = TRUE
```

## `links neighbors` Command

```
vaultmind links neighbors <id-or-path> [--depth N] [--min-confidence low|medium|high] [--max-nodes N] [--json]
```

Default values: `--depth 1`, `--min-confidence low`, `--max-nodes 200`.

### JSON Response

```json
{
  "start_id": "proj-payment-retries",
  "nodes": [
    {
      "id": "proj-payment-retries",
      "distance": 0
    },
    {
      "id": "concept-idempotency",
      "distance": 1,
      "edge_from": {
        "source_id": "proj-payment-retries",
        "edge_type": "explicit_relation",
        "confidence": "high",
        "weight": 0
      }
    }
  ],
  "max_nodes_reached": false
}
```

### Human-readable Output

```
proj-payment-retries (depth 0)
  → concept-idempotency (explicit_relation, high) depth 1
  → person-rend (explicit_relation, high) depth 1
  ← proj-billing-dashboard (tag_overlap, low) depth 1
3 nodes (max 200)
```

### Command Wiring

- `cmd/links_neighbors.go` — thin command, resolves target via `graph.Resolver`, calls `Traverse`, formats output
- Registered under the existing `links` parent command (`cmd/links.go`)
- Uses existing `app.links.*` config prefix for vault and json flags; adds `depth`, `min_confidence`, `max_nodes`

## Query Function

```go
// internal/query/neighbors.go

// NeighborsResult is the JSON response for links neighbors.
type NeighborsResult struct {
    StartID         string           `json:"start_id"`
    Nodes           []NeighborNode   `json:"nodes"`
    MaxNodesReached bool             `json:"max_nodes_reached"`
}

type NeighborNode struct {
    ID       string         `json:"id"`
    Distance int            `json:"distance"`
    EdgeFrom *NeighborEdge  `json:"edge_from,omitempty"`
}

type NeighborEdge struct {
    SourceID   string  `json:"source_id"`
    EdgeType   string  `json:"edge_type"`
    Confidence string  `json:"confidence"`
    Weight     float64 `json:"weight"`
}

// Neighbors runs a BFS traversal and returns the formatted result.
func Neighbors(resolver *graph.Resolver, db *index.DB, input string, depth int, minConfidence string, maxNodes int) (*NeighborsResult, error)
```

## Testing Strategy

### Unit tests

- **BFS basic:** start + direct neighbors at depth 1
- **BFS depth 2:** reaches 2-hop neighbors
- **BFS cycle:** cyclic graph (A→B→A via related_ids) doesn't loop
- **BFS max-nodes:** cap at 5 on a connected graph, verify `max_nodes_reached`
- **BFS min-confidence:** filter out low edges, verify only high/medium returned
- **BFS no neighbors:** isolated node returns just itself
- **BFS unresolved target:** nonexistent ID returns error
- **Confidence ordering:** verify `meetsConfidence` helper

### Integration tests

- **Full traversal on test vault:** verify real edges traversed
- **Command smoke test:** `links neighbors` with JSON output

### Coverage target

85%+ for new code.

## Design Decisions

### DD-1: Traversal in graph package

**Choice:** BFS lives in `internal/graph/traverse.go`.

**Rationale:** It's a graph algorithm. `memory recall` (P3c) calls the same traversal and enriches with frontmatter. Clean separation: graph traverses, memory enriches.

### DD-2: Bidirectional traversal

**Choice:** Both outbound and inbound edges are followed.

**Rationale:** SRS says `links neighbors` returns the "neighborhood." A→B via `related_ids` should make B a neighbor of A, and A a neighbor of B. Treating the graph as undirected for exploration matches how knowledge graphs work.

## File Inventory

| File | Change |
|------|--------|
| `internal/graph/traverse.go` | New: `Traverse`, types, confidence helper |
| `internal/graph/traverse_test.go` | New: BFS tests |
| `internal/query/neighbors.go` | New: `Neighbors` query function, result types |
| `internal/query/neighbors_test.go` | New: query function tests |
| `cmd/links_neighbors.go` | New: `links neighbors` subcommand |
| `internal/config/commands/links_config.go` | Modify: add depth, min-confidence, max-nodes options |
| `.ckeletin/pkg/config/keys_generated.go` | Regenerated |
