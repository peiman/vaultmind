---
id: decision-bfs-with-visited-set
type: decision
status: accepted
title: "Mandate BFS with visited set for all graph traversals"
created: 2026-04-03
vm_updated: 2026-04-03
tags: [graph, correctness]
related_ids:
  - concept-semantic-networks
source_ids: []
---

# Mandate BFS with Visited Set for All Graph Traversals

## Decision

All graph traversal in VaultMind — retrieval, activation spreading, context pack assembly — must use breadth-first search (BFS) with an explicit visited set. No traversal may use DFS or omit cycle detection.

## Rationale

**Cycle prevention.** The [[semantic-networks|Semantic Networks]] graph is not guaranteed to be a DAG. Notes may link to each other (A → B → A). Without a visited set, DFS would loop infinitely or require an arbitrary depth cap that silently truncates results.

**Distance tracking requires BFS.** [[spreading-activation|Spreading Activation]] weights decay with hop distance. BFS guarantees that the first time a node is reached, it is via the shortest path, so its distance-based activation score is maximal and correct. DFS would assign arbitrary distances depending on traversal order.

**Determinism.** BFS over a sorted adjacency list produces a deterministic traversal order for a given seed, making test output stable and results reproducible across runs.

**Simplicity of the visited set contract.** Every traversal function takes a `visited map[string]bool` as input; callers can pre-populate it to exclude nodes from retrieval. This is a clean, composable API that DFS with a stack does not naturally support.

## Trade-offs Accepted

BFS has higher peak memory than DFS for wide graphs. At vault scale (thousands of notes), this is not a concern.
