# VaultMind Dogfooding Evaluation

**Date:** 2026-04-07
**Activity:** Built a curated research knowledge base on LLM + human memory using VaultMind CLI as the primary tool.
**Vault growth:** 42 → 88 domain notes (+46), 10 → 30 sources, 16 → 34 concepts, 0 → 0 unresolved links.

---

## Summary

VaultMind was used as the sole knowledge management tool for a research sprint covering 6 threads: human memory foundations, LLM memory architectures, retrieval systems, knowledge graphs, long-context methods, and evaluation benchmarks. Every note was created, indexed, searched, and retrieved using VaultMind CLI commands.

**Overall verdict:** The core workflow (index → search → recall → context-pack) works well. Three bugs were discovered and fixed during the session. The tool is functional for real research work.

---

## Wins

### Search Works Well at Scale
Searching "memory retrieval" across 88 notes returns 51 matches with meaningful normalized scores (0.0–1.0). The BM25 normalization (Session 04 fix) makes scores immediately interpretable. Results are well-ranked — Associative Memory and Encoding Specificity correctly appear above less relevant matches.

### Context-Pack is the Killer Feature
`context-pack concept-rag --budget 4096` assembles 49 context items totaling 4,075 tokens. Priority ordering works correctly: explicit relations get body text, inferred edges get frontmatter only. The token budget is respected precisely. An agent receiving this payload gets a comprehensive, relevance-ranked knowledge package.

### Memory Recall Graph is Rich
`memory recall concept-multi-store-model --depth 2` reaches 54 nodes through explicit relations, alias mentions, and tag overlap. The graph correctly connects the Multi-Store Model to Working Memory, Episodic Memory, Semantic Memory, Context Pack, and the Atkinson-Shiffrin source paper. The inferred edges (alias mentions, tag overlap) add genuine associative signal.

### Doctor is Now a Real Diagnostic Tool
The doctor command caught every issue we encountered: unresolved links, Obsidian-incompatible wikilinks, and phantom duplicate links. The structured output with `unresolved_link_details` and `incompatible_link_details` is actionable — it shows exactly what's wrong and how to fix it.

### Incremental Indexing is Fast
Adding notes one thread at a time, incremental indexing took <200ms per run (skipping unchanged notes). Full rebuilds of 88 notes complete in ~300ms. No performance issues at this scale.

---

## Bugs Found and Fixed

### Bug 1: Phantom Unresolved Links from Duplicate Wikilinks
**Severity:** High — data integrity issue
**Discovery:** `[[RETRO]]` and `[[RAG]]` showed as unresolved despite the target notes existing.
**Root cause:** When a note references `[[RETRO]]` twice in its body, the parser inserts two link rows. With `dst_note_id = NULL`, SQLite's unique index treats each NULL as distinct, storing both. `ResolveLinks` resolves the first copy but the second hits the unique constraint via `UPDATE OR IGNORE` and stays permanently unresolved.
**Fix:** Deduplicate links by `(dst_raw, edge_type)` in `StoreNote` before inserting.
**Commit:** `e473e18`

### Bug 2: Missing Filename-Stem Resolution
**Severity:** High — broke all Obsidian-compatible wikilinks
**Discovery:** After converting `[[Context Pack]]` to `[[context-pack|Context Pack]]` for Obsidian compatibility, ALL wikilinks became unresolvable. VaultMind resolved by ID, title, and alias — but not by filename stem (`context-pack`).
**Root cause:** `ResolveLinks` had 3 passes (ID, title, alias) but no pass matching against the filename portion of the note's path. Obsidian's `[[filename|display]]` convention stores `dst_raw = "filename"`, which doesn't match any existing resolution tier.
**Fix:** Added 4th resolution pass: `WHERE path LIKE '%/' || dst_raw || '.md'`.
**Commit:** `c97d840`

### Bug 3: No Detection of Obsidian-Incompatible Wikilinks
**Severity:** Medium — silent data quality issue
**Discovery:** Every wikilink in the vault used `[[Title Case]]` format which works in VaultMind but not in Obsidian (Obsidian resolves by filename, not title).
**Fix:** Added `obsidian_incompatible_links` diagnostic to the `doctor` command. Flags resolved wikilinks where `dst_raw` doesn't match the target's filename stem, with `suggested_fix` showing the correct `[[filename|Display]]` format.
**Commit:** `c97d840`

---

## Friction Points (Not Bugs, But Opportunities)

### 1. No Bulk Link Rewriting
When we discovered the Obsidian-incompatible wikilink issue, there was no VaultMind command to fix it across the vault. We had to write a custom Python script. A `vaultmind lint --fix-links` command that rewrites `[[Title]]` to `[[filename|Title]]` would be valuable.

### 2. CLI Command References vs Knowledge Concepts
The note `spreading-activation.md` linked to `[[Memory Recall]]` — intending to reference the CLI command, not a knowledge concept. VaultMind has no way to distinguish "this is a command reference" from "this is a knowledge link." A convention like backtick-code for commands (`\`memory recall\``) vs wikilinks for knowledge is needed and should be documented.

### 3. Tests Coupled to Live Vault
Multiple tests broke when we added research notes because they hardcoded expectations about the vault's content (e.g., `UnstructuredNotes >= 1`). Tests should use a dedicated fixture vault (`test/fixtures/testvault/`) with stable, known content. This decoupling was started but not completed.

### 4. No `note create` from CLI with Inline Content
Creating notes required writing files directly. `note create` works with templates but there's no way to pipe content from an agent into a new note. Something like `echo "body" | vaultmind note create --type concept --title "Foo" --stdin` would improve the agent workflow.

### 5. Doctor Doesn't Detect Command References as Wikilinks
The `[[Memory Recall]]` link resolved to a non-existent path (`_path:Memory Recall.md`) and was counted as "resolved." Doctor should flag links that resolve to `_path:` pseudo-IDs as warnings — they point to files that don't exist in the vault.

---

## Metrics

| Metric | Before | After | Delta |
|--------|--------|-------|-------|
| Domain notes | 42 | 88 | +46 |
| Concepts | 16 | 34 | +18 |
| Sources (papers) | 10 | 30 | +20 |
| People | 8 | 8 | 0 |
| Decisions | 5 | 5 | 0 |
| Projects | 2 | 2 | 0 |
| Unresolved links | 15 | 0 | -15 |
| Obsidian-incompatible links | 138 | 0 | -138 |
| Bugs found | — | 3 | — |
| Bugs fixed | — | 3 | — |
| New tests added | — | 3 | — |

---

## Recommendations for v2

Based on this dogfooding session:

1. **Add `vaultmind lint --fix-links`** — bulk rewrite wikilinks to Obsidian-compatible format
2. **Add `--stdin` to `note create`** — let agents pipe content into new notes
3. **Decouple tests from live vault** — complete the `test/fixtures/testvault/` migration
4. **Flag `_path:` pseudo-ID resolutions in doctor** — these indicate dead references
5. **Document the wikilink convention** — `[[filename|Display Text]]` for knowledge, backtick-code for CLI commands
6. **Consider a pre-commit hook** — run doctor on the vault and fail if incompatible links are found
