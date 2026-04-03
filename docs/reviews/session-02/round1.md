# Expert Panel — Round 1: Independent Reviews

**Date:** 2026-04-03 | **Document reviewed:** VaultMind SRS v3 (post-fix, incorporating Session 01 findings)

---

## Panel Members

| # | Expert | Specialty |
|---|--------|-----------|
| 1 | Dr. Elena Vasquez | Cognitive neuroscience, long-term memory |
| 2 | Marcus Chen | Obsidian power user, vault architecture |
| 3 | Jordan Blackwell | Devil's advocate, systems architecture |
| 4 | Dr. Priya Sharma | Knowledge graphs, graph databases |
| 5 | Alex Novak | AI agent systems engineering |
| 6 | Kai Nakamura | AX (AI Experience) design |
| 7 | Dr. Lena Hoffmann | AI/LLM memory systems research |

---

## 1. Dr. Elena Vasquez — Cognitive Neuroscience Review

### Assessment of Session 01 Fixes

The confidence/provenance rename, BFS mandate, and max-nodes cap are all adequate responses to the issues raised. These changes improve the model's scientific honesty without requiring architectural changes.

### Remaining Gaps

- **Context-pack sort order incorrect:** `context-pack` sorts results by `updated` descending. This is wrong — the correct order is weight descending, then recency as a tiebreaker. Sorting by recency alone as the primary key discards associative strength entirely.
- **No frequency weight on `alias_mention` edges:** Alias mention edges carry no count of how many times the mention appears. A note that mentions a concept once is treated identically to one that mentions it thirty times. Occurrence frequency is a direct proxy for associative strength.
- **`tag_overlap` threshold unvalidated on small vaults:** The threshold of 1.0 is not tested against vaults with fewer than fifty notes. At small scale, the tier collapses to empty — every tag set is too sparse for any pair to score above the threshold.

### New Observations

The IDF score invalidation problem (Expert 4 raises this as well) has a direct memory-science implication: stale IDF scores cause recently deleted notes to continue exerting retrieval influence. This is the computational analog of a perseveration error — the system "remembers" associations that no longer exist.

### Top Recommendations

1. **Weight-then-recency sort in `context-pack`:** Primary sort key must be associative weight (edge confidence × IDF contribution); recency serves only as a tiebreaker.
2. **Alias occurrence count as edge weight:** Record how many times each alias surface form appears in the source note body and expose this as `mention_count` on the `alias_mention` edge.
3. **Vault-size-adaptive `tag_overlap` threshold:** Make the threshold a function of vault size, or provide a minimum floor so that small vaults always produce at least some tag-overlap edges.

---

## 2. Marcus Chen — Obsidian Practitioner Review

### Assessment of Session 01 Fixes

YAML 1.2 strict mode is properly fixed. The normalize time-strip is now opt-in via `--strip-time`, which is the correct call. The `updated` field conflict has been documented in the risks table.

### Remaining Gaps

- **Template `${variable}` still collides with Templater JS mode:** The Session 01 fix changed syntax from `{{variable}}` to `${variable}`. The rationale text argues this avoids Templater collision, but Templater's JavaScript mode uses `${variable}` as standard template literal syntax. The rationale is self-contradicting — the chosen delimiter is not safe.
- **`updated` conflict only in risks table, not in schema spec:** Implementers reading the schema section will not see the warning. Carrying the conflict note into the schema spec where `updated` is defined is necessary for it to be acted upon.
- **Block scalar round-trip behavior unspecified for `normalize`:** The spec still does not state what `frontmatter normalize` does when it encounters `|` or `>` block scalars. Implementers will make inconsistent choices.

### New Observations

The template delimiter problem is actually worse in MCP context. MCP tool call bodies are often constructed by the LLM using string interpolation. A `${variable}` delimiter in a schema that LLMs are asked to populate will cause template literal injection in any JavaScript-based MCP host.

### Top Recommendations

1. **Change template delimiter to a truly unique syntax:** Neither `{{}}` nor `${}` is safe. Use a delimiter that does not appear in any Obsidian plugin, Dataview query, or JavaScript template literal — for example `[[%variable%]]` or a prefixed form like `vm:variable`.
2. **Carry the `updated` conflict into the schema spec:** Add a `⚠ Conflict risk` note directly in the `updated` field definition, not only in the risks appendix.
3. **Specify normalizer block-scalar behavior:** Add an explicit rule: if a frontmatter value is a block scalar, `normalize` must preserve the exact scalar style (literal or folded) and must not reformat its content.

---

## 3. Jordan Blackwell — Devil's Advocate Review

### Assessment of Session 01 Fixes

The plan sequencing fix is correct and surgical — resolving targets before execution rather than during is the right approach. The atomicity documentation is honest about best-effort rollback.

### Remaining Gaps

- **"All-or-nothing" in `15-nonfunctional` contradicts "best-effort" in sections 10 and 14:** The nonfunctional requirements section still uses atomic language while the mutation and plan sections correctly describe best-effort rollback. A reader who reads section 15 first will carry a false expectation.
- **Template syntax rationale self-contradicting:** See Expert 2. The collision argument used to justify `${variable}` is incorrect.
- **`--commit` semantics split between sections 07 and 10:** The flag is introduced in section 07 and given additional semantics in section 10. An implementer reading only one section will produce an incomplete or inconsistent implementation.

### New Observations — New #1 Failure Mode: Index Staleness

This is the most serious unaddressed risk in the current spec. After any mutation (note create, frontmatter set, note delete), the SQLite index is stale until the next `index` call. An agent performing a read-modify-write-recall sequence will execute the recall step against pre-mutation state and receive no signal that the data is stale.

`meta.index_hash` does not help here — it tells the caller when the index was last built, but it does not tell the caller whether a mutation that just occurred has been incorporated into the index. The agent has no way to distinguish "index is current" from "index is stale because I just wrote."

Additional unaddressed failure modes:

- **Git hook failure has no recovery path:** If the post-write Git hook fails, the file has been written but the commit has not been made. The spec does not describe how to detect or recover from this state.
- **`updated` is still a day-one killer:** Despite being documented in the risks table, the field has not been renamed. Every vault using an auto-update plugin will experience hash conflicts immediately.

### Top Recommendations

1. **Add index staleness signal:** After every mutation response, include `index_stale: true` in the response envelope. Separately, `index auto` mode should re-index after every write by default.
2. **Reconcile atomicity language:** Remove "all-or-nothing" from section 15 or add a parenthetical that explicitly defines it as best-effort with file-copy rollback.
3. **Consolidate `--commit` semantics** into a single canonical section with a cross-reference note in the other location.

---

## 4. Dr. Priya Sharma — Knowledge Graph Review

### Assessment of Session 01 Fixes

BFS mandate, composite indexes, uniqueness constraint, max-nodes cap, and delete-before-reinsert are all correctly incorporated. These were the most structurally important fixes and they have been applied accurately.

### Remaining Gaps

- **Unique index breaks on NULL `dst_note_id`:** SQLite treats NULL as not equal to NULL. A unique index on `(src_note_id, dst_note_id, edge_type)` will allow duplicate rows whenever `dst_note_id` is NULL (unresolved links). This will silently create duplicate unresolved-link rows on each re-index.
- **`memory recall` response missing `max_nodes_reached` flag:** The max-nodes cap was added but the response shape has not been updated to signal when it fires. Callers cannot distinguish a naturally small result from a truncated one.
- **BFS confidence filtering order unspecified:** The spec adds confidence filtering to BFS traversal but does not state whether filtering happens before or after the max-nodes cap is applied. The two orderings produce different result sets.
- **Tag overlap IDF scores not invalidated on note deletion:** When a note is deleted, its term frequencies are removed from the index but the IDF scores of all tags that appeared in that note are not recomputed. Subsequent `tag_overlap` edge weights are computed against stale IDF values.
- **Missing `frontmatter_kv(key, value_json)` index:** Frontmatter key-value queries lack a dedicated index. Agents filtering by frontmatter field values will fall back to full-table scans.
- **No self-edge prevention for `tag_overlap`:** Nothing prevents a note from generating a `tag_overlap` edge to itself. The BFS implementation must skip self-edges, but the schema has no CHECK constraint to enforce this at the storage layer.

### New Observations

The NULL uniqueness gap in the `links` table is a correctness bug, not a performance issue. It will manifest silently — the index will report no error, but the graph will contain duplicate edges that inflate traversal results without any warning.

### Top Recommendations

1. **Add `max_nodes_reached` flag to `memory recall` response:** Callers must know when results are truncated. Add a boolean field to the `meta` envelope of the recall response.
2. **`dst_raw` parser invariant + self-edge CHECK constraint:** Add a partial unique index that covers only rows where `dst_note_id IS NOT NULL`, and add a `CHECK (src_note_id != dst_note_id)` constraint on the `links` table.
3. **`frontmatter_kv` index + IDF invalidation scope:** Add the missing index on frontmatter key-value pairs, and specify that note deletion triggers recomputation of IDF scores for all tags that appeared in the deleted note.

---

## 5. Alex Novak — Agent Systems Review

### Assessment of Session 01 Fixes

All nine Session 01 fixes have landed correctly. The BFS mandate, max-nodes cap, `--frontmatter-only`, `note mget` addition, batch-read support, and structured error objects are all present and correctly specified.

### Remaining Gaps

- **`updated` field still not renamed (highest severity):** This has been documented as a risk but not resolved. Every agent interacting with a vault that uses an auto-update plugin will encounter hash conflict errors. This is not a documentation problem — it requires renaming the field to `vm_updated` or a similarly namespaced key.
- **`mget` and `apply` response shapes missing from section 09:** The response shapes document covers the original commands but has not been updated to include the two new commands added in response to Session 01 feedback. Implementers will have no canonical shape to implement against.
- **YAML 1.2 requirement is scattered:** The YAML 1.2 strict mode requirement appears in multiple places but not in the data model spec where the frontmatter schema is defined. An implementer reading only the data model spec will not find it.
- **Plan rollback still best-effort:** Already noted by Expert 3. From the agent perspective, a plan that partially executes and then rolls back should return a structured failure response indicating which steps completed before rollback. The current spec does not specify this response shape.
- **`--force` audit trail incomplete:** The spec states that `--force` bypasses checksum verification but does not specify whether this bypass is logged. Agents need a way to audit which writes were forced.

### Top Recommendations

1. **Rename `updated` to `vm_updated` now:** Do this in the schema spec, the frontmatter normalize command, and all examples. Every session of delay increases the migration cost.
2. **Add `mget` and `apply` response shapes to section 09:** These are new commands with non-trivial response structures. They must be in the canonical response shapes document.
3. **Specify YAML 1.2 in the data model spec:** Add a normative statement to the frontmatter schema section: "All YAML frontmatter must be parsed and serialized in YAML 1.2 strict mode."

---

## 6. Kai Nakamura — AX (AI Experience) Design Review

### Agent Discoverability

- **No cold-start command:** An agent opening VaultMind for the first time requires four separate calls to understand the vault state (index status, type registry, schema version, available commands). There is no single `vault status` command that returns all of this in one response.
- **No capabilities manifest:** Agents cannot introspect which optional features are enabled (Git integration, watch mode, Dataview support) without probing each individually.
- **Edge type semantics require out-of-band knowledge:** An agent that has not read the SRS does not know what `alias_mention` or `tag_overlap` edges mean. Edge types should carry a `description` field in `schema list-types` output.

### Idempotency

- **No mutation receipt / `write_hash`:** Mutation responses do not include a hash of the resulting note state. An agent that retries a failed write cannot determine whether the first attempt actually succeeded.
- **No idempotency key on plans:** Multi-step plans have no client-supplied idempotency key. A plan submitted twice (due to network timeout) will execute twice.
- **`note_create` not retry-safe:** Creating a note with a duplicate title produces an error rather than returning the existing note. Agents that retry on timeout will fail on the second attempt.

### Token Efficiency

- **`--max-nodes` default of 200 is too high:** A recall result with 200 nodes consumes roughly 4,000–8,000 tokens depending on frontmatter size. The default should be 25, with 200 available as an explicit override.
- **`note get` should default to body-off:** Most agent use cases need frontmatter only. Returning full body by default wastes context. The `--frontmatter-only` flag exists but should be the default, with `--include-body` as the explicit opt-in.
- **Five memory commands for one concept:** `memory recall`, `memory related`, `memory context-pack`, `links neighbors`, and `links graph` all address the question "what is connected to this note?" This should collapse to three commands with mode flags.
- **`blocks` field is noise for agents:** The generated-region blocks field in note responses is never needed by agents performing semantic tasks. It should be omitted by default.
- **Warnings need structure:** Warnings in the response envelope are bare strings. Agents cannot programmatically handle a warning they cannot parse. Warnings must be structured: `{code, message, affected_field}`.

### MCP-Specific Concerns

- **`--commit` is a hidden side effect:** In MCP context, a tool call that writes a file and commits to Git is performing two distinct actions under one tool name. These should be separate tools (`note_write` and `vault_commit`) or `--commit` must be explicitly surfaced as a tool parameter, not a config default.
- **`resolve` is redundant for MCP:** In MCP context, the host can provide a note ID directly. The `resolve` command adds a round-trip with no benefit. It should be optional or collapsed into other commands.
- **No MCP resources defined:** VaultMind's vault content is a natural fit for MCP resource exposure. Notes, types, and the schema should be available as MCP resources, not only as tool call results.
- **Merge list semantics undefined:** When `frontmatter merge` receives a list value for a field that already contains a list, the spec does not define whether the result is a union, a replacement, or an append.

### Top Recommendations

1. **`vault status` cold-start command:** Returns index status, type registry, schema version, enabled features, and note count in a single call. This is the entry point for any agent session.
2. **`write_hash` in mutation responses:** Every mutation response must include a hash of the resulting note frontmatter state. This enables idempotent retry without a separate read call.
3. **Body-off default on `note get`:** Flip the default to frontmatter-only; require `--include-body` to receive prose content.
4. **Structured warnings:** Replace bare-string warnings with `{code, message, affected_field}` objects throughout the response envelope.
5. **Collapse memory commands 5 → 3:** Merge `memory recall` + `memory related` into one command with a `--mode` flag; merge `links neighbors` + `links graph` into `links traverse`; keep `memory context-pack` as-is.

---

## 7. Dr. Lena Hoffmann — AI/LLM Memory Systems Review

### Graph Memory vs. Semantic Memory

The structured graph model is correct and well-suited for relational queries (what is linked to this note, what shares these tags). It fails at fuzzy semantic recall — the retrieval pattern required by LLM agents that need to ask "what do I know that is relevant to this problem" rather than "what is connected to this specific node."

VaultMind's intended use case maps to the MemGPT model of archival long-term memory. Under that model, the system is responsible for storing, surfacing, and compressing agent memory across sessions. The current spec addresses storage and basic retrieval but does not address the other two responsibilities.

### Missing Memory Architecture Components

- **No working memory tier:** MemGPT distinguishes working memory (active context window content) from archival memory (external store). VaultMind has no concept of a working memory tier. Agents must manage this themselves, which defeats the purpose of the memory layer.
- **No importance scoring:** The Park et al. (2023) generative agent architecture assigns importance scores to memories at write time using an LLM call. Without importance scoring, all notes are treated as equally relevant. High-signal notes (decisions, key findings) compete equally with low-signal notes (meeting transcripts, raw dumps).
- **No access frequency / recency tracking:** Retrieval frequency and recency are the two strongest predictors of memory accessibility in both human cognitive models and practical agent memory systems. Neither is tracked.
- **No summarization / compression:** Long-running agents accumulate memory. Without a summarization pass, archival storage grows without bound and retrieval quality degrades as the signal-to-noise ratio drops.
- **No query-relative ranking:** `context-pack` ranks by `updated` timestamp. There is no mechanism to rank results by relevance to the query that triggered the recall. A query-relative ranking function (even a simple BM25 over frontmatter fields) would substantially improve retrieval quality.
- **No reflection / synthesis mechanism:** Generative agent architectures include a reflection step where high-level insights are derived from lower-level memories. VaultMind has no analog — no command for deriving or storing synthesized knowledge from an existing note set.
- **`tag_overlap` threshold is a hard cliff:** The 1.0 threshold produces binary in/out decisions. A soft scoring function (normalized overlap score from 0.0 to 1.0) would be more appropriate for retrieval ranking and would eliminate the small-vault emptiness problem.

### New Observations

The context-pack `updated` sort problem and the tag_overlap threshold problem are not minor tuning issues — they are symptoms of the absence of a retrieval relevance model. A well-specified memory system should define how relevance is computed, not leave it implicit in sort order.

### Top Recommendations

1. **Agent access log table:** Add an `agent_access_log(note_id, accessed_at, query_context)` table. This enables frequency and recency tracking, which are prerequisite inputs for any principled retrieval ranking function.
2. **`salience` field in frontmatter schema:** Add an optional numeric `salience` field (0.0–1.0) to the frontmatter schema. This supports author-set importance and LLM-assigned importance scoring at write time.
3. **`summary` generated region + `memory summarize` command:** Add a `<!-- vm:summary -->` generated region to the note schema and a `memory summarize` command that populates it. This is the minimum compression mechanism required for long-running agents.
4. **Semantic expansion extension point:** Add a documented extension point (a no-op by default) for embedding-based candidate expansion before graph traversal. This allows downstream implementers to add vector search without forking the retrieval architecture.
5. **Reflection note type:** Add `reflection` to the type registry as a first-class note type. Reflection notes are LLM-generated syntheses derived from a set of source notes. Formalizing this type enables the retrieval layer to weight synthesized knowledge appropriately.
