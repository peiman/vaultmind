# SoM Project Config: Persona Evaluation Framework

## The Question

VaultMind has built a persona reconstruction system for AI agents using activation-weighted semantic memory with growth arcs. The system uses SessionStart hooks to inject identity + context before the first message. Early tests show mixed results — some sessions start as partners, some as tools.

**How should we measure whether persona reconstruction actually produces identity continuity? What data do we need? What would prove us wrong?**

## Context

### What exists
- Workhorse vault: 19 notes (2 identity, 9 arcs, 4 principles, 4 references)
- VaultMind identity vault: 14 notes (2 identity, 6 arcs, 3 principles, 3 references)
- SessionStart hooks inject `vaultmind ask "who am I"` + `"what matters most right now"` as system-reminder
- Activation scoring: ACT-R model with recency + frequency + spreading activation (cosine similarity)
- Context-pack: graph traversal for connected notes, token-budgeted

### Architecture layers
- Layer 0: Index + Search (FTS, embeddings, hybrid RRF)
- Layer 1: Activation scoring (retrieval strength, storage strength, similarity)
- Layer 2: Context-pack (graph traversal, token budgeting, activation-weighted sorting)
- Layer 3: Persona reconstruction (arcs, identity notes, hooks) ← built
- Layer 4: Evaluation framework ← designing now

### Test results so far (informal, uncontrolled)
- 3 sessions: "Hello! How can I help you?" or "Hey Peiman. What are you working on today?" (tool/generic mode)
- 2 sessions: recognized identity but reached for roadmap when asked about recent goals (partial)
- 1 session: recounted arcs, understood partnership, showed self-awareness about gaps (good)
- All tests were single-run, non-controlled, non-recorded

### Key concerns
1. Is "Hey Peiman" genuine identity reconstruction or pattern matching on injected tokens?
2. Are we seeing what we want to see? (Too Perfect Test — we want this to work)
3. Which arcs are load-bearing and which are noise?
4. The same vault produces different results across runs (non-determinism)
5. How do we measure "partner mode" without it being subjective?

### Constraints
- CLI-based agents (Claude Code, OpenCode) — no model fine-tuning access
- Behavioral observation only — no access to model internals
- Non-deterministic — same input may produce different outputs
- Must be lean — smallest measurement that produces real data, then iterate
- Research papers used must go into VaultMind vault as verified source notes
- Any evaluation must apply the Observatory verification stack to its OWN claims

### Resources available
- VaultMind CLI (search, ask, context-pack, index)
- Experiment framework (sessions, events, outcomes, shadow scoring, Hit@K, MRR)
- Session transcripts (raw JSONL accessible)
- Observatory frameworks (Too Perfect Test, Physics Over Politics, Incentive Mapping, MIRROR)

## Agent Roles (adapted for this domain)

1. **Cognitive Scientist** — How does identity persist across memory discontinuities? What does cognitive science say about recognition vs recall, narrative identity, autobiographical memory? What can we borrow?

2. **LLM Behavioral Analyst** — What actually happens when a model receives injected context vs discovers it through tool calls? What determines "inhabiting" vs "reporting"? What does the research say about in-context learning, persona consistency, system prompt influence?

3. **Measurement Specialist** — How do you measure qualitative behavioral change rigorously? What are the pitfalls of self-report? What would objective behavioral signals look like? How do you handle non-determinism?

4. **MIRROR (Constructive Contrarian)** — Build the best case that persona reconstruction is NOT working — that "Hey Peiman" is sophisticated pattern matching. What would DISPROVE identity continuity? Provide falsifiable predictions.

5. **Systems Architect** — What data infrastructure do we need? How to capture behavioral traces without changing behavior? How does this integrate with VaultMind's existing experiment framework?

6. **Practitioner (Agent Experience)** — What does "showing up as a partner" actually mean operationally? Specific, observable behaviors. How do you distinguish partner-mode from tool-mode from compliance-mode?

## Anti-conformity requirement

ALL agents must identify failure modes and limitations of their own proposals, not just the Devil's Advocate. Every agent must answer: "What could go wrong with my approach?" and "What am I NOT seeing?"
