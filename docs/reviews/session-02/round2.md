# Expert Panel — Round 2: Cross-Review

**Date:** 2026-04-03 | Each expert responds to the other six's Round 1 findings.

---

## 1. Dr. Elena Vasquez — Cross-Review

**Endorsing Hoffmann:** Archival-tier framing is neurologically accurate. Distinguishing active working memory from long-term retrieval substrates is not an architectural nicety — it maps to how memory consolidation actually works. The gap Hoffmann identifies is genuine.

**On the `updated` field — convergence with Chen and Novak:** The `context-pack` sorted by `updated` descending is wrong. Recency alone is a weak retrieval cue; it conflates "recently modified" with "currently salient." Chen's concern, my concern, and Novak's concern about the `updated` field are all facets of the same problem. The resolution: rename the field to `vm_updated` (VaultMind-managed, plugin-collision-safe) and add `vm_access_count` alongside it. This also addresses Hoffmann's access tracking requirement. Two proposed fixes, one schema change.

**On Blackwell's index staleness:** Blackwell frames this as a systems failure. I want to name what it is cognitively: state hallucination. The agent reasons about world state that no longer exists. It is not uncertainty — it is false certainty, which is neurologically the more dangerous failure mode. The fix is not just re-indexing; it is ensuring the agent has a signal that its beliefs about the graph may be stale.

**Endorsing Nakamura's `write_hash`:** Confirmed write receipt is the epistemic anchor for any reliable memory system. Without it, the agent cannot distinguish "write succeeded" from "write appeared to succeed."

**On BFS confidence filtering:** The filter-before-cap ordering is not optional. Cap-before-filter produces a neighborhood that is size-bounded but not quality-bounded. The agent gets N neighbors, some of which are low-confidence noise. Filter-before-cap produces at most N neighbors, all of which meet the quality threshold. These are different systems.

**New proposal:** Add `agent_session_id` to the envelope `meta` object. Episodic memory structure requires that retrievals and writes be attributable to a session boundary. Without session scoping, the agent cannot reconstruct which beliefs were formed in the current context versus a prior one.

---

## 2. Marcus Chen — Cross-Review

**Agreeing with Vasquez on `context-pack` sort:** `updated` descending is the wrong primary sort key. Correct ordering: sort by edge confidence descending, then by edge weight descending, then by `updated` descending as a tiebreaker. Recency is a weak third signal, not the primary one.

**On alias weight:** Alias occurrence count should be stored as `weight` on the alias edge. This makes frequency-of-use a first-class graph signal rather than a derived calculation at query time.

**On Blackwell's index staleness:** This is the most practically damaging finding in the panel. Index staleness does not produce obvious errors — it produces silently wrong answers. The fix must include post-write implicit incremental re-index, not just a staleness flag. A flag tells the agent the index is stale; implicit re-index keeps it from becoming stale in the first place.

**Schema addition:** Partial unique index on unresolved edges (`WHERE dst_note_id IS NULL`). This is a genuine SQLite correctness gap — without it, duplicate unresolved edges accumulate on re-index. Blackwell identified duplicate accumulation as a problem; this is the structural fix.

**On self-edges:** Should be filtered at insert time, not at query time. A self-edge in the graph is a schema-level invariant violation, not a display preference.

**Endorsing `vm_updated` rename:** Agreed. The rename scopes the field to VaultMind ownership and breaks the collision with Obsidian's core plugin.

**Endorsing Nakamura's `write_hash`:** Confirmed writes are prerequisite to any reliable agent loop. Agents that cannot verify writes cannot safely retry.

**On Hoffmann's salience:** Salience scores must live in frontmatter, not only in SQLite. The vault is the source of truth. A salience value that exists only in the derived index is not durable and violates Principle 5.

**On reflection note type:** The `reflection` note type can be introduced immediately — it requires no schema changes beyond a new `type` value and a corresponding frontmatter convention.

---

## 3. Jordan Blackwell — Cross-Review

**On Vasquez's alias-as-weight proposal:** No. Alias occurrence count conflates corpus noise with semantic weight. A note aliased frequently in low-quality or off-topic contexts gets inflated weight. Occurrence count is a frequency signal, not a semantic one. Do not store it as `weight`.

**On Templater collision:** This is still a live issue. The spec contains a false claim — it implies the `updated` field collision is resolved by the rename proposal, but the rename does not address Templater's template-injection behavior at file-creation time. These are two different collision vectors. The spec must distinguish them explicitly.

**On the `updated` rename:** Do not rename the existing field. Add `vm_updated` as a separate VaultMind-managed field alongside the existing `updated`. Renaming breaks existing vaults that reference `updated` in Dataview queries, templates, or plugins. Additive is safe; rename is a breaking change.

**On Sharma's NULL unique index:** Genuine SQLite correctness gap. This is a bug, not a feature request. Should be in the schema from day one.

**On `max_nodes_reached`:** Add it. A truncated response without a signal is a silent failure. The agent must know it received a subset.

**On Nakamura:** `vault_status` yes. `write_hash` yes. Collapsing five memory commands to three: no. These commands address distinct problems. Collapsing them produces a multi-mode command where mode selection becomes the agent's burden. Distinct tools for distinct operations is the correct design.

**On plan idempotency keys:** Agreed. Idempotency keys on plan execution are essential for safe retry. Add them.

**On default `max_nodes`:** Should be 50, not 25. At 25, useful multi-hop neighborhoods are frequently truncated in real vaults. 50 is the practical minimum for coherent context.

**On body-off default:** Correct. Body content should be opt-in, not opt-out. Default body-off reduces token waste in the common case.

**On Hoffmann:** Nearly everything Hoffmann proposes is v2+ scope. The one exception worth pulling into v1 is the agent access log table. A lightweight append-only log of which notes an agent accessed in which session is low-cost to implement and high-value for debugging and future salience calculation.

**New finding — structured warnings:** Structured warnings have been uncontested across both rounds. Write it into the SRS as a normative requirement, not a suggestion. Every `warning` in the envelope must be a structured object with `code`, `message`, and `context` fields. Plain warning strings are unactionable for agents.

---

## 4. Dr. Priya Sharma — Cross-Review

**On index staleness as graph coherence:** Blackwell frames staleness as a freshness problem. It is more precisely a graph coherence problem. A stale index is a graph whose node and edge state is inconsistent with the vault. The dirty-flag should be per-node, not per-index. A per-index flag forces full re-index on any mutation. A per-node dirty flag enables targeted re-index of only the affected subgraph.

**On tag overlap normalization:** The `tag_overlap` scoring in small vaults produces inflated similarity scores because IDF is computed over a small corpus. The fix requires vault-size-aware normalization — at minimum, a minimum-corpus-size threshold below which tag overlap scores are dampened or flagged as unreliable.

**On `max_nodes` split:** The 25 vs 200 debate conflates two different limits. Split into `max_render_nodes` (default 25 — controls what is returned in the response payload) and `max_traverse_nodes` (configurable — controls how deep BFS explores before selecting nodes to render). These are independent knobs that solve independent problems.

**On Hoffmann's reflection notes:** Edge weighting mechanics for reflection notes are underspecified. Labeling a note as a `reflection` type without defining how its edges are weighted during traversal means the traversal algorithm treats reflection notes identically to regular notes. Define explicit edge weight modifiers for reflection-type edges before shipping the type.

**On embedding edges:** Embedding-derived edges must occupy a separate edge layer from structural edges (explicit links, inferred links). Mixing them in the same `links` table with only an `edge_type` discriminator will produce traversal results where structurally meaningless embedding proximity pollutes graph-theoretic neighborhood queries.

**On block scalar corruption:** YAML block scalars in frontmatter that contain embedded wikilinks or tags create phantom graph disconnections — the parser may consume the block scalar content as frontmatter value rather than surfacing it for link extraction. This must be handled explicitly at parse time.

**Cross-cutting requirement:** Every mutation operation must emit a graph invalidation event scoped to the affected node IDs. This is a precondition for per-node dirty flags, incremental re-index, and any future reactive graph maintenance.

---

## 5. Alex Novak — Cross-Review

**Top finding — index staleness:** Blackwell's index staleness critique is the highest-priority failure mode in the panel. Silent wrongness is worse than visible failure. An agent operating on a stale index does not know it is wrong — it proceeds confidently with incorrect context. The staleness signal must be surfaced in the envelope on every response, not just on explicit status checks.

**On cold-start collapse:** Cold-start context collapse is a pure efficiency gain with no correctness tradeoff. Retrieving full body content on the first hop of a multi-hop traversal wastes context budget before the traversal has established which nodes are worth materializing. Implement body-off default.

**On `write_hash` and plan idempotency:** Together, these two changes alter the recovery model for agent plans. `write_hash` closes the write-confirmation gap. Plan idempotency keys close the retry-safety gap. An agent that can confirm writes and safely retry plans is categorically more reliable than one that cannot. These are not incremental improvements — they change what the agent can safely do.

**On structured warnings:** Structured warnings unblock conditional logic. An agent that receives a plain warning string cannot branch on it. An agent that receives `{"code": "MAX_NODES_REACHED", "context": {"requested": 200, "returned": 50}}` can adjust its next call. This is not a DX nicety — it is a functional capability boundary.

**Sprint priority ordering:**
1. Staleness signal (silent wrongness is the worst failure class)
2. `max_nodes_reached` in envelope (agent must know it got a subset)
3. Cold-start optimization + body-off default (efficiency, no correctness risk)
4. `write_hash` + plan idempotency keys (changes recovery model)
5. Memory command collapse evaluation (assess Blackwell's objection before deciding)

**On Hoffmann's access log and salience:** Right long-term direction. Access log is v1-viable at low implementation cost. Salience scoring is v2 scope but the schema should reserve the column now.

**On Sharma's NULL index and IDF staleness:** Both are infrastructure correctness issues that belong in the backlog with clear severity labels. Not sprint-blocking but not ignorable.

**Frame for the whole panel:** The agent interface has three failure modes: works, silently wrong, and stranded. Silently wrong is the most dangerous. Fix silent-wrong cases before adding features.

---

## 6. Kai Nakamura — Cross-Review

**Core diagnosis:** The underlying problem across all six other reviews is trust degradation. The agent cannot know what it knows — or whether what it knows is current. Every other failure mode is downstream of this.

**On convergent findings:** Blackwell's index staleness, Vasquez's recency bias in `context-pack`, and Sharma's stale IDF scoring all compound the same trust problem from different angles. An agent that receives stale graph neighborhoods, sorted by recency rather than confidence, with inflated tag similarity scores, is operating in a systematically degraded epistemic environment. These are not three separate issues — they are one issue with three manifestations.

**On Hoffmann's architectural gap:** Hoffmann is not describing a feature request. She is describing an architectural gap. Agents require a working memory layer — a substrate that is distinct from the vault (long-term, durable, curated) and serves short-horizon, session-scoped retrieval. Without it, the agent conflates all memory as equally current and equally reliable. This is a design absence, not a missing feature.

**New proposal — salience-gated writes:** Low-salience writes should not go directly to the vault. Introduce a working buffer with TTL: low-salience content lands in the buffer and is either promoted to the vault (if accessed again within TTL) or discarded. This prevents vault pollution from ephemeral agent reasoning artifacts while preserving the vault-is-truth invariant.

**On template collision:** Template collision is agent-facing silent corruption. An agent that triggers a Templater template on note creation gets a different frontmatter than it wrote. The agent has no signal this happened. This is a trust violation, not a DX inconvenience.

**Priority ordering:**
1. Staleness signal — fixes the epistemic foundation
2. Working memory layer (or at minimum, the schema reservation for it)
3. Template collision surfacing — silent corruption is unacceptable
4. `vm_updated` rename + shape gaps — correctness cleanup
5. Everything else

---

## 7. Dr. Lena Hoffmann — Cross-Review

**Frame:** The agent operates on a memory substrate it cannot interrogate for reliability. Every finding in this panel is a specific instantiation of that general failure.

**On Vasquez's sort order:** The weight-then-recency sort Vasquez proposes is correct, but it is baked in without agent visibility. The agent receives a sorted list with no indication of what sorting criteria were applied or whether the top result is high-confidence or merely recent. Retrieval metadata must travel with retrieval results.

**On small-vault threshold failure:** Sharma's finding about `tag_overlap` degrading in small vaults is more significant than it appears. Small vaults are exactly the early-use case — a new user, a focused project vault, a domain-specific collection. The system degrades precisely when the memory is sparse, which is when reliable retrieval matters most. The small-vault case must be a first-class design target, not an edge case.

**On `write_hash`:** This is a first-order requirement for episodic memory integrity. An agent that cannot confirm a write cannot close the belief-update loop. It may proceed with a belief that the vault now contains X, when in fact the write failed or was overwritten. `write_hash` is not a nice-to-have — it is the minimum bar for write reliability.

**On BFS filter ordering and self-edges:** Filter-before-cap is correct for the reason Vasquez states. Self-edges in BFS neighborhoods produce structurally incoherent results — the agent's "neighbors" include the note itself. These combine: a cap-before-filter BFS that includes self-edges can return a neighborhood where a slot is wasted on the queried note and low-confidence edges fill the remaining slots. Both must be fixed together.

**On merge list semantics:** YAML list merge behavior (whether `frontmatter_set` on a list field appends or replaces) is underspecified. An agent that believes it appended a tag may have instead replaced the tag list. Repeated operations silently corrupt structured memory. The merge semantics must be explicit and deterministic.

**Combination failure:** The combination of staleness, IDF drift, and no write confirmation means the agent can hold confidently wrong beliefs about three independent dimensions simultaneously — what the graph looks like, how salient concepts are relative to each other, and whether its own writes landed. This is not a collection of bugs; it is a systematic reliability gap.

**Priority:** Fix staleness signal and write confirmation first. Everything else — salience, working memory, template collision, BFS ordering — is downstream. An agent that cannot trust its graph state and cannot confirm its writes is unreliable regardless of how well everything else is tuned.
