# Expert Panel — Summary of Findings

**Date:** 2026-04-03 | **Orchestrator summary** after 2 rounds with 6 experts.

> Full transcripts: [Round 1](review-panel-round1.md) | [Round 2](review-panel-round2.md)

---

## Consensus Findings (agreed by 4+ experts)

These issues had strong cross-panel agreement and should be treated as high-priority fixes.

### 1. `--json` must default when stdout is not a TTY
**Raised by:** Novak, Torres, Blackwell, Chen
**Status:** Unanimous agreement. Agents will pipe output and silently get unparseable human-readable text. This is a contract bug, not a feature request. Detection via `isatty(stdout)`.

### 2. Structured error objects in JSON envelope
**Raised by:** Torres, Novak, Blackwell
**Round 2 reinforcement:** All agree `"errors": ["string"]` is insufficient. Must be `[{"code": "...", "message": "...", "field": "..."}]`. Error codes exist for mutations but aren't surfaced in the envelope structure.

### 3. `updated` field collision with Obsidian auto-update plugins
**Raised by:** Chen | **Endorsed by:** Novak (called it "single highest-severity finding"), Blackwell, Vasquez
**Impact:** Plugins like `update-time-on-edit` rewrite `updated` on every save, causing hash conflicts that refuse VaultMind writes. Breaks the agent write-confirm loop. Day-one problem.
**Recommended fix:** Use a distinct field name (`vm_updated`) or document the conflict prominently with plugin configuration guidance.

### 4. Plan create-then-configure sequencing bug
**Raised by:** Blackwell | **Endorsed by:** Novak (upgraded to blocker), Sharma
**Issue:** Plan with `note_create` followed by `frontmatter_set` targeting the new note fails — targets are resolved before any execution. Common agent pattern made impossible.
**Fix:** Either defer target resolution to per-operation execution time, or allow `note_create` operations to register their IDs for subsequent operations within the same plan.

### 5. Template `{{variable}}` syntax collides with Templater
**Raised by:** Chen | **Endorsed by:** Torres, Novak
**Impact:** Templater auto-processes VaultMind templates on sight, corrupting placeholders.
**Fix:** Change delimiter to `${variable}`, `<%variable%>`, or document that templates must be excluded from Templater's watch paths.

### 6. Cycle detection unspecified for graph traversal
**Raised by:** Sharma | **Endorsed by:** Blackwell, Novak
**Impact:** `related_ids` and wikilinks can form cycles. Traversal at depth >1 will loop without a visited set. Correctness requirement.
**Fix:** Mandate visited-set BFS in spec for all depth>0 traversals.

### 7. `note get` needs `--frontmatter-only`
**Raised by:** Novak | **Endorsed by:** Torres, Sharma, Vasquez
**Impact:** Agents doing bulk sweeps burn context window on full note bodies. Sharma notes this changes the traversal/materialization boundary in the memory engine.

### 8. "Confidence" is provenance, not epistemic uncertainty
**Raised by:** Vasquez | **Endorsed by:** Novak, Sharma, Blackwell
**Disagreement on fix:** Vasquez wants rename to `edge_source`. Torres warns rename breaks stable output policy. Chen suggests `provenance_tier`.
**Consensus fix:** Keep `confidence` field for backward compatibility. Add companion `edge_source` field. Document that confidence = provenance tier, not epistemic certainty.

### 9. Missing database indexes
**Raised by:** Sharma | **No disagreement**
**Missing:** `links(edge_type)`, `links(confidence)`, `links(src_note_id, resolved)`, `notes(type)`, `aliases(note_id)`.
**Impact:** 500ms depth-2 recall target at risk on vaults >5,000 notes.

### 10. `--max-nodes` cap needed on graph traversals
**Raised by:** Sharma | **Endorsed by:** Torres, Novak
**Impact:** Hub note at depth 2 = fan-out explosion. `memory recall` has no bound. `context-pack` has token budget, but recall does not.
**Fix:** Add `--max-nodes` flag (default 200) on `links neighbors`, `memory recall`.

---

## Strong Findings (2-3 experts, no opposition)

### 11. Inline `#tags` in body text not indexed
**Raised by:** Chen | **Endorsed by:** Sharma (graph correctness issue — IDF scores inflated)
**Fix:** Index body `#tags` into `tags` table, or explicitly document as out-of-scope.

### 12. No batch-read command
**Raised by:** Novak | **Endorsed by:** Torres
**Fix:** Add `note mget` accepting list of IDs, returning frontmatter-only by default.

### 13. `apply` needs stdin support
**Raised by:** Novak | **Endorsed by:** Torres (use `-` as plan-file arg per Unix convention)
**Impact:** Essential for MCP/sandboxed environments where filesystem write isn't guaranteed.

### 14. Plan atomicity is best-effort, not true
**Raised by:** Blackwell | **Endorsed by:** Vasquez, Novak (partial)
**Impact:** Process killed mid-plan leaves partial writes. Pre-operation copies are not atomic.
**Fix:** Document honestly as best-effort. Consider WAL-style journal for plan execution.

### 15. YAML boolean coercion hazard
**Raised by:** Chen | **Endorsed by:** Blackwell, Sharma
**Impact:** `yes`/`no`/`true`/`false` silently coerced by YAML 1.1 parsers. Corrupts `status` fields and aliases.
**Fix:** Require YAML 1.2 strict mode parser. Document in implementation spec.

### 16. `links` table lacks uniqueness constraint
**Raised by:** Sharma (Round 2) | Unique finding
**Impact:** Incremental re-indexing without deleting old edges produces duplicates. Silently corrupts traversal and overlap scores.
**Fix:** Add composite unique index or mandate delete-before-reinsert.

### 17. `--field` list-value syntax unspecified
**Raised by:** Novak, Torres
**Impact:** How does `--field related_ids=[a,b]` work? Agents will fail on first list-field attempt.
**Fix:** Specify exact syntax (e.g., YAML scalar parsing) or require plan files for list fields.

### 18. Schema introspection command missing
**Raised by:** Novak
**Fix:** Add `vaultmind schema list-types --json` returning type registry.

### 19. `--watch` + mutations = concurrency violation
**Raised by:** Blackwell | **Endorsed by:** Torres
**Impact:** Agent mutating while `--watch` re-indexes violates single-writer assumption.
**Fix:** Process lock, or document `--watch` as human-only mode.

### 20. `--force` on dataview render needs audit trail
**Raised by:** Blackwell (Round 2)
**Fix:** Forced overwrites must emit `warning` in envelope and be ineligible for `--commit` without confirmation.

---

## Contested Findings (experts disagree)

### `--diff` should merge into `--dry-run`
**For:** Torres | **Against:** Chen, Blackwell, Vasquez
**Verdict:** Keep separate. They answer different questions. `--dry-run` = "would this apply?", `--diff` = "what exactly changes?".

### Temporal decay / retrieval frequency tracking
**For:** Vasquez | **Against:** Blackwell (violates Principle 5), Chen (harmful for evergreen notes)
**Verdict:** Defer to v2. Consider opt-in recency weighting in context-pack ordering, but do not track agent access patterns in the derived index.

### Numeric edge weights on all edges (vs three confidence tiers)
**For:** Vasquez, Sharma | **Against:** Blackwell (false precision), Chen (calibration instability)
**Verdict:** Keep three tiers for v1 API. `tag_overlap` already has numeric `weight`. Add numeric `weight` field to storage layer as future-proofing, but don't expose in v1 response payloads.

### Merge `explicit_embed` into `explicit_link`
**For:** Sharma, Blackwell | **Against:** Chen (embeds are semantically distinct — transclusion vs navigation)
**Verdict:** Keep as separate type. Obsidian users distinguish them. Add `target_kind: embed|link` attribute for traversal convenience.

### `resolve` and `index` as root-level commands
**For moving:** Torres, Chen | **Neutral:** Others
**Verdict:** Keep at root for v1 (they're high-frequency commands). Add category headers to `--help` output.

---

## Deferred to v2 (valid but not blocking)

- Temporal decay / recency-weighted retrieval (Vasquez)
- Contextual query mode for `memory related` (Vasquez)
- Salience scoring via PageRank variant (Vasquez, Sharma)
- `frontmatter backfill` migration command (Chen)
- Numeric edge weights in API responses (Sharma, Vasquez)
- `note list` command (Torres, Blackwell)
- API server MCP integration details (Novak)

---

## Top 10 Action Items (Priority Order)

| # | Action | Severity | Effort |
|---|--------|----------|--------|
| 1 | Fix plan create-then-configure sequencing (defer resolution to execution) | Blocker | Medium |
| 2 | Default `--json` when not TTY | High | Low |
| 3 | Structured error objects in JSON envelope | High | Low |
| 4 | Document `updated` field plugin conflict + mitigation | High | Low |
| 5 | Change template variable syntax (avoid Templater collision) | High | Low |
| 6 | Mandate visited-set BFS for cycle detection | High | Low |
| 7 | Add `--frontmatter-only` to `note get` and `memory recall` | High | Low |
| 8 | Add missing database indexes | High | Low |
| 9 | Add `--max-nodes` cap to graph traversals | Medium | Low |
| 10 | Require YAML 1.2 strict mode parser | Medium | Low |

---

## Overall Assessment

The SRS is architecturally sound. The core design — immutable IDs, canonical vault, derived index, plan-then-apply mutations, Git policy matrix — is well-reasoned. The panel found no fundamental design flaws that require rethinking the architecture.

The issues found are in three categories:

1. **Contract bugs** (items 1-3): The JSON output contract has gaps that will cause agent failures in production. These are low-effort, high-impact fixes.

2. **Ecosystem friction** (items 4-5, 11, 15): Real-world Obsidian vaults will hit immediate compatibility issues. These are documentation + small design changes.

3. **Missing specs** (items 6-10): Graph traversal, index performance, and CLI flags need tightening. These are spec additions, not redesigns.

Blackwell's meta-critique stands: **validate against a real vault before writing code.** Build the scanner and parser first, index a messy 5,000-note vault, and let the failures refine the spec. The current spec is coherent but untested.
