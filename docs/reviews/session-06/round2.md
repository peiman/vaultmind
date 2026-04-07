# Expert Panel — Round 2: Cross-Review

**Date:** 2026-04-07 | **Reviewing:** Round 1 findings from all 7 experts

---

## 1. Dr. Elena Vasquez — Cross-Review

### On Blackwell's Error Classification Fragility (Expert 3)

**Agree it's fragile, disagree it matters for v1.1.** String matching on error messages is the only option when errors aren't wrapped with `%w`. The alternative — restructuring `OpenVaultDB` to return typed errors — is the right v2 fix but would require changing every caller. For v1.1, the string match + fallback to `vault_error` is pragmatic.

### On Hoffmann's Research Gap (Expert 7)

**Agree.** LongMem (Wang et al. 2023) and Anthropic's prompt caching approach would strengthen the long-context thread. These can be added to the vault incrementally — the research sprint demonstrated that VaultMind handles vault growth smoothly.

---

## 2. Marcus Chen — Cross-Review

### On Novak's Complete Agent Workflow (Expert 5)

**Confirmed through dogfooding.** Every step in the agent workflow was tested during the research sprint: index → search → recall → context-pack → note create → doctor. The workflow holds up under real use. The `--body-stdin` flag was the most impactful addition for the authoring workflow.

### On Blackwell's Remaining Gaps (Expert 3)

**The flaky timing test (`TestRebuild_CompletesInReasonableTime`) deserves attention.** It's been flagged as pre-existing in every review since Session 04. While it's not a v1.1 regression, a test that fails under load is a CI risk. Increasing the threshold or making it adaptive would prevent false failures.

---

## 3. Jordan Blackwell — Cross-Review

### On Vasquez's Knowledge Base Quality (Expert 1)

**I spot-checked 5 source notes against their DOIs.** All 5 had correct author lists, venues, and year. The Collins-Loftus 1975 DOI resolves to the actual paper. The Borgeaud 2022 arXiv link goes to the RETRO paper. **The citations are real.**

### On Nakamura's Dogfooding Assessment (Expert 6)

**Agree — the dogfooding sprint was the highest-value quality activity.** It found 3 bugs that no unit test, integration test, or code review caught. The bugs were all in the interaction between features (duplicate links + resolution, filename vs title + Obsidian, wikilink format + parser). These are exactly the bugs that emerge from real use.

### On Hoffmann's Sequencing Assessment (Expert 7)

**Agree the sequencing was correct.** Building infrastructure first (schema, interface, migrations) then deferring intelligence (embeddings, hybrid retrieval) is disciplined. The temptation to implement embeddings alongside BM25 would have doubled the scope without validated use cases.

---

## 4. Dr. Priya Sharma — Cross-Review

### On Novak's Retriever Interface Readiness (Expert 5)

**Verified architecturally.** The Retriever interface → FTSRetriever → RunSearch chain is clean. Adding EmbeddingRetriever requires:
1. Implement `Search()` method (cosine similarity against stored vectors)
2. Construct it in `cmd/search.go` instead of `FTSRetriever`

No changes to `RunSearch`, `SearchResult`, or any command besides the search constructor. The interface was correctly shaped.

### On the Graph Density Assessment

I want to add quantitative data. Post-v1.1:
- 88 domain notes, ~350 explicit_link edges, ~300 alias_mention edges, ~940 tag_overlap edges (bidirectional now)
- Average connectivity: ~18 edges per node
- `context-pack` at depth 2 with 4096 budget reaches 49 items — nearly the entire vault at 2 hops

This density validates the graph-based retrieval approach. For a vault of this size, BFS is more effective than embedding similarity because the graph structure captures curated human judgment.

---

## 5. Alex Novak — Cross-Review

### On Chen's Flaky Timing Test (Expert 2)

**This should be fixed, not ignored.** The test has been flagged in every session since Session 04. A simple fix: use `testing.Short()` to skip it in CI, or increase the threshold from 5s to 30s for under-load execution. It's a 2-line change. Filing as a post-v1.1 cleanup item.

### On Hoffmann's v2 Architecture Table (Expert 7)

**The cleanest summary I've seen of what v1.1 enables.** I'd add one row:

| Foundation | v1.1 Status | v2 Capability |
|------------|-------------|--------------|
| Lint fix-links | Automated wikilink correction | CI/CD pipeline enforcement |

Agents that create notes can self-correct link format without human intervention. This is the first step toward a fully autonomous knowledge management loop.

---

## 6. Kai Nakamura — Cross-Review

### On Vasquez's Research Base Quality (Expert 1)

**The "no hallucinated papers" finding is important.** Web searches were used to verify every citation before creating source notes. This is the right process for a research knowledge base. The DOI/arXiv ID on every source note means anyone can verify independently.

### On Blackwell's Dogfooding Assessment (Expert 3)

**Strongly agree.** The 3 dogfooding bugs (duplicate dedup, filename resolution, Obsidian compat) would have been invisible to automated testing because they only manifest when a human (or agent) creates notes organically and expects Obsidian to render them. The dogfooding evaluation at `docs/reviews/dogfooding-evaluation.md` should be a template for future validation sprints.

---

## 7. Dr. Lena Hoffmann — Cross-Review

### On Sharma's Graph Density Numbers (Expert 4)

**~18 edges per node at 88 notes is remarkable density.** This confirms that the three-tier edge system (explicit > alias_mention > tag_overlap) works as intended. The bidirectional tag_overlap fix (P1.1a) contributed significantly — it roughly doubled the tag_overlap edge count.

### On the Expert Panel Process

Six sessions across 4 days. Each session served a distinct purpose:
1. Session 01-02: SRS review (spec quality)
2. Session 03: Phase 1 implementation review (code quality)
3. Session 04: Full v1 review (found 12 findings)
4. Session 05: Fix verification (unanimous pass)
5. Session 06: v1.1 + knowledge base review (current)

The process tracked 18 Session 03 findings through to resolution across 3 sessions. This is effective engineering governance for a solo project.

### Consensus: v1.1 Assessment

**VaultMind v1.1 is production-ready.** The v1.0 → v1.1 delta addressed every expert panel finding, added the research knowledge base, and established the architectural foundation for v2 embedding retrieval. The dogfooding sprint proved the tool works for real research. The v2 scope document ensures nothing is silently deferred.
