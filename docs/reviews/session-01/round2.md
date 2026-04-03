# Expert Panel — Round 2: Cross-Review

**Date:** 2026-04-03 | Each expert responds to the other five's Round 1 findings.

---

## 1. Dr. Elena Vasquez — Cross-Review

**Endorsing Jordan Blackwell:** Plan atomicity critique is the most important finding. Best-effort undo dressed as atomicity. For any stateful system, partial writes that corrupt scheduling state are worse than no write.

**Endorsing Priya Sharma:** Three-confidence-level critique directly reinforces my concern. Associative strength is continuous, not categorical. Numeric edge weights (0.0–1.0) are the right model. Her `is_inferred` column and `via_id` field for path tracing are exactly what's needed to distinguish direct recall cues from reconstructed ones.

**Endorsing Alex Novak:** Memory recall payloads being too large maps to interference — oversized context degrades signal quality of the retrieval cue. Leaner payloads aren't just token efficiency; they're cognitive fidelity.

**Challenging Marcus Chen:** His findings are real but he treats the `updated` field hash conflict and datetime stripping as schema problems. They're retrieval timing integrity problems. If timestamps get coerced by plugins, temporal ranking calculations are wrong.

**Challenging Sam Torres:** Disagree on merging `--diff` into `--dry-run`. Previewing what will change versus confirming you want to commit are two distinct cognitive steps that should be surfaced separately.

**What nobody addressed:** None of the five mentioned that the system has no per-card or per-user calibration parameters for any future decay model. Individual differences in forgetting rates are substantial.

---

## 2. Marcus Chen — Cross-Review

**On Dr. Vasquez:** Technically correct, largely irrelevant for this system. VaultMind is not a cognitive memory simulator. Temporal decay in a knowledge vault is harmful — evergreen notes on foundational concepts should NOT decay. "Retrieval-induced forgetting" would actively corrupt a reference system. However, she's right that "confidence" conflates provenance with epistemics. Use `provenance_tier`. Her "encoding specificity / context-blind retrieval" concern has real Obsidian application — `context-pack` has no notion of which vault context you were in. Legitimate gap.

**On Jordan Blackwell:** Strongest review in the panel. "Source of truth" vs checksum contradiction is real. Preview-before-hash-verify race window is not theoretical in hot vaults with iCloud/Obsidian Sync. O(notes × aliases) validates my indexing performance concern.

**On Priya Sharma:** Merging `explicit_embed` into `explicit_link` would DESTROY information Obsidian users rely on. Embeds are transclusion, not navigation — distinct semantic meaning. Disagree with consolidation. Numeric weights are premature for v1 — three tiers with clear provenance are more defensible. Her `via_id` column is a legitimate schema gap.

**On Alex Novak:** `note mget`, `--frontmatter-only`, `--stdin` for apply — all correct. `--json` defaulting when not TTY is right Unix convention.

**On Sam Torres:** `resolve` and `index` as orphan verbs — valid. His `--diff` into `--dry-run` proposal is WRONG. `--dry-run` means "do nothing"; `--diff` means "show what would change." Distinct affordances. Keep them separate.

**Net verdict:** Blackwell and Novak raised the sharpest new material. Vasquez's critique is scientifically valid but misapplied. Sharma's numeric weights are premature. Torres's `--diff` consolidation is a regression.

---

## 3. Jordan Blackwell — Cross-Review

**On Vasquez — Partial agree, partial hard no:** "Confidence as provenance" rename — do it, it's just vocabulary cleanup. But temporal decay and `recall_count` — hard no. VaultMind is not spaced-repetition. Adding decay couples derived index to agent behavior history, violating Principle 5 (derived artifacts are never canonical truth). An agent firing recall 50 times doesn't change a note's epistemic weight.

**On Marcus Chen — Everything actionable:** Every finding is real. The `updated` collision is highest-severity. YAML boolean coercion is a silent data corruption vector zero others caught. Inline `#tags` gap means the `tags` table is incomplete by design — spec must say so explicitly. Migration path is valid but acceptable deferral for v1.

**On Priya Sharma:** `explicit_embed` should be collapsed — adds no graph question it answers differently from `explicit_link`. Disagree strongly on numeric weights everywhere — three categorical levels are deliberate. Numeric weights create false precision. `tag_overlap` already has numeric score. `is_inferred` boolean is legitimate. `via_id` is sound but only if BFS is mandated.

**On Alex Novak:** `--json` defaulting when not TTY is a bug in the contract, not a feature request. Agents will pipe and get human output by accident. `note mget` is a real gap. `--field` list syntax: specify it, don't redesign it. Structured error objects in envelope — correct, plain strings are unactionable.

**On Sam Torres:** `--diff` merging into `--dry-run` is wrong. They answer different questions. Keep both. `note list` is a genuine omission. `index --watch` structured events — agree completely.

**What nobody caught:** The `--force` flag on `dataview render` creates a silent override path. Any forced overwrite must emit a `warning` in the envelope and be ineligible for `--commit` without explicit confirmation. The safety model has an unaudited escape hatch.

---

## 4. Dr. Priya Sharma — Cross-Review

**On Vasquez — Right instinct, wrong implementation:** Salience via inbound-link-count is underspecified. Raw in-degree conflates hub notes (daily logs, index pages) with genuinely important concepts. Need weighted PageRank variant where edge weight factors confidence tier. Her instinct is right but implementation detail is wrong. Agree that "confidence is provenance" — should rename to `inference_strength` or `edge_class`.

**On Marcus Chen — Graph integrity failure:** Inline `#tags` not indexed means the IDF weighting formula computes against incomplete corpus. Specificity scores systematically inflated for tags appearing in bodies but not frontmatter. This is a graph correctness issue, not just DX. His YAML coercion concern also affects `aliases` table — boolean-coerced alias values break entity resolution.

**On Jordan Blackwell:** Alias O(n × m) is a graph index design problem. Fix: trigram index on `aliases.alias_normalized` for sub-linear lookup. He identifies symptom; the fix is structural. His "create-then-configure impossible" implicates graph consistency — edges from unresolved `related_ids` are silently absent.

**On Alex Novak:** `--frontmatter-only` is a graph traversal optimization. Allows memory engine to traverse 2-3 hops before materializing body content. Changes the traversal/materialization boundary.

**On Sam Torres:** `links neighbors` vs `memory recall` distinction maps to a real architecture question: same graph, different serialization? Or different traversal semantics? Must be resolved before implementation.

**What ALL five missed:** The `links` table has NO uniqueness constraint on `(src_note_id, dst_note_id, edge_type)`. During incremental re-indexing, if edges aren't deleted before reinsertion, duplicates accumulate silently. Corrupts traversal results and inflates overlap scores. Need composite unique index or explicit delete-before-reinsert contract.

---

## 5. Alex Novak — Cross-Review

**On Vasquez:** Aligned on `confidence` naming problem. An agent can't distinguish `high` on `alias_mention` from `high` on `explicit_relation` without out-of-band knowledge. Salience/decay are valid but lower priority for v1. Defer decay.

**On Marcus Chen:** The `updated` field collision is the SINGLE highest-severity finding in the entire panel. An agent that writes `updated` then gets a hash conflict from a plugin has a broken write-confirm loop — can never reliably verify mutations landed. Template syntax collision is also agent-critical.

**On Jordan Blackwell:** Plan atomicity point is partially correct but overstated. However, his create-then-configure sequencing gap is the most important finding nobody else addressed. An agent issuing a plan with `note_create` then `frontmatter_set` on that same note will fail because targets are resolved before execution. This is a genuine sequencing bug. The plan spec must either (a) defer resolution to per-operation execution time, or (b) document that created notes must appear last with validation enforcing ordering.

**On Priya Sharma:** Numeric weights worth adding to storage layer but agents don't need them in v1 response payloads. `via_id` directly relevant — agents can't reconstruct paths. `is_inferred` boolean is cleaner than deriving from `edge_type`.

**Updating my Round 1:** Adding Blackwell's create-then-configure gap as a blocker-level finding.

---

## 6. Sam Torres — Cross-Review

**On Alex Novak — Largely aligned:** TTY detection is not optional. `note mget` with `--frontmatter-only` cuts N calls to 1. `apply` should accept `-` as plan-file arg for stdin (Unix convention, cleaner than `--stdin` flag). Disagree partially on `meta` — keep it default, add `--no-meta` opt-out.

**On Marcus Chen — Obsidian collisions are CLI problems too:** The `updated` collision means `frontmatter set updated` could be immediately overwritten by a plugin before next read. `doctor` should surface detected plugin conflicts. Template error messages must distinguish VaultMind failures from Templater passthrough.

**On Jordan Blackwell:** `--watch` concurrent with mutations is exactly the race condition the SRS promises to avoid. CLI should either refuse `--watch` with a process lock, or document it as human-facing only.

**On Priya Sharma:** `--max-nodes` belongs on `links neighbors`, `memory recall`, and `context-pack`. Graph-aware cap solves different problem than token-aware `--budget`. Both needed.

**On Vasquez:** Renaming `confidence` to `edge_source` would break every response shape — violates stable output policy. Right move is additive: keep `confidence`, add `edge_source` as companion field.

**What the panel collectively missed:** The `--field key=value` syntax on `note create` for multi-word or list values. `--field tags=billing,payments` — is it a string, a YAML list, or an error? Must specify: YAML scalar syntax or separate `--field-list` flag.
