# Expert Panel — Round 1: Independent Reviews

**Date:** 2026-04-07 | **Reviewing:** VaultMind v1.1 (40 commits, 1849 tests, 88 domain notes)

---

## 1. Dr. Elena Vasquez — Cognitive Neuroscience / Memory Review

### Research Knowledge Base Quality

The vault now covers the major human memory concepts with real, verified sources. The 6 research threads are well-chosen:
- **Human memory foundations** (episodic/semantic memory, levels of processing, schema theory, multi-store model, consolidation, SOAR) — all correctly attributed with DOIs
- **LLM architectures** (RETRO, Memorizing Transformers, Voyager, ALM survey) — accurate arXiv IDs
- **Retrieval** (DPR, REALM, Self-RAG, FiD, ColBERT, HyDE) — correct venues and findings
- **Knowledge graphs** (GraphRAG, KGAG, Lost in the Middle) — timely and relevant
- **Benchmarks** (LongBench, RAG vs Long Context) — grounded in real experimental findings
- **Long-context** (Infini-Attention, Ring Attention) — cutting-edge 2024 work included

**No hallucinated papers found.** All source notes I spot-checked have correct author lists, venues, and DOIs. The "Memory Research Knowledge Base" project note correctly maps the three research areas.

### Memory Summarize — Cognitive Assessment

The `memory summarize` command assembles material without calling an LLM. This is the right design — it separates data assembly from synthesis, letting the agent do the cognitive work. The reflection note type with `trigger`, `insight`, and `source_ids` fields mirrors the structure of metacognitive reflection in human memory.

### Context-Pack Depth

The `--depth N` parameter enables multi-hop packing. The priority formula `distance*10 + edgePriority` is a reasonable first pass — it ensures distance dominates, with edge confidence as a tiebreaker. This is analogous to activation decay in spreading activation models: closer nodes get more activation regardless of connection strength.

---

## 2. Marcus Chen — Obsidian Practitioner / Vault Review

### Wikilink Convention — Enforced and Working

The vault is clean: 0 unresolved links, 0 Obsidian-incompatible links. The `[[filename|Display Text]]` convention is documented and enforced by `doctor` + `lint fix-links`. This is a genuine improvement — before v1.1, the vault had 138 incompatible links.

### Vault Health

88 domain notes with rich cross-linking. Running `memory recall concept-rag --depth 2` reaches 54 nodes — the graph is well-connected. The tag overlap and alias mention inferred edges add real associative signal.

### Reflection Template

The reflection template has the right structure: Trigger, Synthesis, Sources. The `source_ids` field in frontmatter means reflection notes automatically link into the knowledge graph via explicit_relation edges. This is clean integration with the existing type system.

### Note Create with Stdin

`--body-stdin` enables the agent workflow: generate content, pipe it in, get a properly templated note. This was a real friction point during the dogfooding sprint — creating notes required writing files directly.

### Lint Fix-Links — The Missing Tool

During dogfooding, we needed a Python script to fix 138 links. Now `lint fix-links` does this natively. The dry-run default is the right choice — show what would change before doing it.

---

## 3. Jordan Blackwell — Devil's Advocate / Systems Review

### Duplicate ID — First File Wins

This is now correct. The `continue` in the indexer skips the second file, preserving the first. The `DuplicateIDs` counter still tracks violations. Session 03 asked for this; it's delivered.

### Error Code Classification

`classifyVaultError` uses string matching on error messages. This is fragile — if the error message changes in `OpenVaultDB`, the classification breaks silently. However, it's documented with a comment explaining why (errors aren't wrapped with `%w`), and the fallback to `vault_error` means agents always get a valid code. Acceptable for v1.1.

### GROUP_CONCAT Separator

The `char(31)` (unit separator) fix is correct. The code review caught the `|` issue and it was fixed before shipping. This is a good example of the review process catching a real data corruption risk.

### Remaining Gaps

- `Rebuild()` / `Incremental()` errors in `cmd/index.go` still bypass JSON (documented in v2 scope)
- `TestRebuild_CompletesInReasonableTime` is flaky under load (pre-existing, not a v1.1 regression)
- No access frequency tracking wired into commands yet (schema is ready, wiring is v2)

These are all documented in `docs/specs/v2-scope.md`. Nothing is silently deferred.

---

## 4. Dr. Priya Sharma — Knowledge Graph Review

### Tag Overlap Bidirectional

Verified: both A→B and B→A edges are inserted. `links out` now works symmetrically for tag_overlap. The count tracking correctly handles partial failures (each insert is independently counted).

### Alias Collision Fix

`aliasToNoteIDs` is `map[string][]string`. When two aliases normalize to the same lowercase ("API" and "api" from different notes), both notes get `alias_mention` edges. This was a subtle data loss bug — now fixed.

### Filename-Stem Resolution (4th Pass)

The 4th resolution pass (`WHERE REPLACE(path, '.md', '') LIKE '%/' || dst_raw`) enables Obsidian-compatible wikilinks. This is architecturally significant: VaultMind now resolves by ID, title, alias, AND filename stem — covering all four ways a note can be referenced.

### Link Deduplication

The `seenLinks` map in `StoreNote` prevents duplicate links from the same source. The key `dst_raw + "\x00" + edge_type` correctly deduplicates by target and edge type. This eliminates the phantom unresolved link bug.

### Graph Metrics

Post-v1.1 vault: 88 domain notes, ~1000+ edges (explicit + alias_mention + tag_overlap). The graph is dense enough for meaningful retrieval. `context-pack concept-rag --budget 4096` assembles 49 context items from a single seed — demonstrating real graph utility.

---

## 5. Alex Novak — Agent Systems Review

### Agent Workflow — Complete

An agent can now:
1. Discover commands via `--help` (improved help text)
2. Index the vault with JSON error handling
3. Search with normalized [0,1] scores and accurate totals
4. Recall subgraphs with correct edge directions
5. Pack token-budgeted context at configurable depth
6. Summarize notes for reflection synthesis
7. Create notes from templates with stdin body
8. Lint and fix wikilinks
9. Get structured error codes for recovery logic

This is a complete agent-facing API. No blind spots remain for common workflows.

### Retriever Interface — Ready for v2

`RunSearch` now accepts `Retriever` interface. `FTSRetriever` is the sole implementation. Swapping in `EmbeddingRetriever` requires zero changes to the search command — only the construction in `cmd/search.go`.

### Error Codes — Actionable

Four granular codes: `vault_not_found`, `config_error`, `database_locked`, `vault_error`. An agent can now:
- Retry on `database_locked`
- Report and halt on `vault_not_found`
- Suggest config fix on `config_error`

### Doctor as Agent Health Check

`doctor --json` returns structured diagnostics: unresolved links, Obsidian-incompatible links, pseudo-ID references, duplicate IDs. An agent can call `doctor` after mutations to verify vault integrity.

---

## 6. Kai Nakamura — AX (AI Experience) Review

### Dogfooding Validated the Tool

The research sprint was the most valuable validation activity: 46 notes created using VaultMind, 3 real bugs found and fixed, 5 friction points identified. The evaluation at `docs/reviews/dogfooding-evaluation.md` is honest and actionable.

### Lint Fix-Links — Agent-Usable

An agent that creates notes with `[[Title]]` format can now self-correct:
```bash
vaultmind lint fix-links --vault ./vault --fix
```
This closes the loop: create → detect → fix.

### Memory Summarize — Good Constraint

The decision to NOT call an LLM in `memory summarize` is the right agent boundary. VaultMind assembles data; the agent synthesizes. This keeps VaultMind focused as a memory tool, not an AI pipeline.

### Context-Pack Depth — Agent Control

`--depth 2` gives agents explicit control over retrieval breadth. The default depth=1 preserves backward compatibility. An agent can start narrow and widen if the initial context is insufficient.

---

## 7. Dr. Lena Hoffmann — AI/LLM Memory Systems Review

### Architecture Readiness for v2

The v1.1 changes lay clean groundwork for v2:

| Foundation | v1.1 Status | v2 Capability |
|------------|-------------|--------------|
| Retriever interface | Wired into RunSearch | Swap FTS → Embedding → Hybrid |
| Embedding column | Migration 003 applied | Store vectors per note |
| Access tracking | Migration 004 applied | ACT-R-style decay scoring |
| Score normalization | BM25 [0,1] min-max | Cross-query sigmoid (v2) |
| Multi-hop packing | --depth N implemented | Broader context windows |
| Reflection type | Defined and templated | Agent metacognition |

The sequencing is correct: infrastructure first (v1.1), intelligence second (v2).

### Research Knowledge Base — Assessment

The 30 source notes cover the essential literature:
- **Foundational:** Ebbinghaus 1885, Bartlett 1932, Atkinson-Shiffrin 1968, Tulving 1972, Craik-Lockhart 1972, Collins-Loftus 1975, Anderson 1983, Newell 1990
- **Modern LLM:** Park 2023 (Generative Agents), Packer 2023 (MemGPT), Shinn 2023 (Reflexion), Borgeaud 2022 (RETRO), Wu 2022 (Memorizing Transformers)
- **Retrieval:** Lewis 2020 (RAG), Karpukhin 2020 (DPR), Guu 2020 (REALM), Asai 2023 (Self-RAG), Khattab 2020 (ColBERT), Gao 2023 (HyDE)
- **Emerging:** Edge 2024 (GraphRAG), Liu 2023 (Lost in the Middle), Munkhdalai 2024 (Infini-Attention)

This is a strong foundation. The gap: no papers on memory consolidation in LLMs (Wang et al. 2023 "LongMem" would fit), and no coverage of Anthropic's or OpenAI's context caching approaches. These are v2 research additions.

### Session 03 → v1.1 Final Resolution

All 18 Session 03 findings are now either resolved or explicitly documented in v2 scope:
- **Resolved:** 16/18 (blockers, high, medium items)
- **Documented in v2:** 2/18 (QueryFullNote 5→4 queries improved but not to 1, path-based excludes done)

The expert panel process has been effective: 6 sessions across 4 days, tracking findings through to resolution.
