---
id: decision-vaultmind-mcp-server
type: decision
status: accepted
title: "VaultMind MCP Server Design"
created: 2026-04-09
vm_updated: 2026-04-09
tags:
  - architecture
  - mcp
  - integration
related_ids:
  - concept-a-rag
  - source-zhang-kraska-khattab-2025
  - concept-memos
source_ids:
  - source-zhang-kraska-khattab-2025
---

# VaultMind MCP Server Design

## Decision

VaultMind will expose a Model Context Protocol (MCP) server with six tools:

1. `vault_search` — keyword and semantic search across all notes
2. `vault_read` — fetch a single note by ID, returning full content and metadata
3. `vault_graph_traverse` — BFS/DFS traversal from a seed note, returning neighbor subgraph
4. `vault_context_pack` — assemble an activation-ranked context bundle for a given query
5. `vault_ask` — combined retrieval and synthesis: search + graph + LLM answer
6. `vault_record_access` — record a note access event, updating activation scores

The server uses pure Go, stdio transport (JSON-RPC over stdin/stdout), and wraps the existing `internal/` packages with no new retrieval logic. All retrieval behavior lives in `internal/`; the MCP layer is a thin tool-dispatch adapter.

## Context

The [[source-zhang-kraska-khattab-2025|RLM implementation]] at `~/dev/ai/RLM` requires a structured knowledge backend that can serve as an external environment for long-context reasoning. An MCP server makes VaultMind usable by any MCP-compatible client: Claude Code, RLM, Cursor, and future agents.

The MCP standard (Anthropic, 2024) defines a JSON-RPC protocol for tool-calling over stdio or HTTP. Stdio transport is appropriate for local-first tools that run on the same machine as the client.

## Rationale

**A-RAG hierarchical interface**: [[concept-a-rag|A-RAG]] (2026) demonstrates that exposing retrieval at multiple granularities (keyword, semantic, chunk-read) lets LLM agents autonomously select the most efficient strategy. VaultMind's six tools extend the A-RAG three-tool model with graph traversal and context-pack, both of which A-RAG lacks.

**MemOS three-tier architecture**: [[concept-memos|MemOS]] (2025) frames activation scoring as a tier-promotion mechanism. VaultMind's `vault_context_pack` and `vault_record_access` tools implement exactly this: recording access updates activation scores, which context-pack uses to promote notes from cold storage to the active context tier.

**MCP as the standard for local tools**: MCP has been adopted by major IDEs (Cursor, VS Code via Claude extension) and is the native protocol for Claude Code tool calls. Implementing MCP means zero integration cost for any future MCP client.

**Wrap don't rewrite**: The existing `internal/retrieval`, `internal/graph`, `internal/activation`, and `internal/ask` packages already implement the retrieval logic. The MCP server adds a dispatch layer only — no new algorithms.

## Consequences

- VaultMind becomes usable by any MCP client without code changes on the client side
- The [[source-zhang-kraska-khattab-2025|RLM system]] can use VaultMind as a knowledge backend via `vault_search`, `vault_graph_traverse`, and `vault_read`
- Claude Code sessions automatically have access to the vault via `vault_context_pack` and `vault_ask`
- `vault_record_access` closes the feedback loop: every retrieval updates activation, making future retrievals more accurate
- The stdio transport means no network port management, no authentication, and no dependency on network availability
- Adding new retrieval capabilities (e.g., hybrid re-ranking, causal graph traversal) requires only adding a new tool definition, not modifying the protocol

See [[source-zhang-kraska-khattab-2025|RLM Paper]] and [[concept-a-rag|A-RAG]] for the research basis for the six-tool design.
