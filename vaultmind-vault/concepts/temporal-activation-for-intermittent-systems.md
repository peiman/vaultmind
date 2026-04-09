---
id: concept-temporal-activation-for-intermittent-systems
type: concept
title: "Temporal Activation for Intermittent Systems"
status: active
created: 2026-04-09
tags:
  - memory
  - activation
  - architecture
  - research-paper
related_ids:
  - concept-base-level-activation
  - concept-power-law-forgetting
  - concept-hebbian-learning
  - concept-act-r
  - source-anderson-schooler-1991
  - proj-vaultmind
---

## Temporal Activation for Intermittent Memory Systems: Adapting ACT-R's Base-Level Learning Equation for CLI Knowledge Tools

### Abstract

ACT-R's base-level activation equation Bi = ln(sum tj^(-d)) assumes continuous wall-clock time, validated against environments where information need evolves steadily (newspapers, email, library checkouts). CLI knowledge tools like VaultMind violate this assumption: they are completely inert between invocations, with usage patterns ranging from hourly to monthly. We analyze seven approaches to temporal activation under intermittent usage, evaluate each against cognitive science theory and practical systems, and propose a compressed-idle-time model that preserves the power-law form while reducing the penalty for system inactivity. We complement this with Bjork & Bjork's dual-strength distinction to prevent catastrophic decay of well-learned items after long hiatuses.

### 1. The Problem

The base-level activation equation from ACT-R (Anderson, 1990) estimates the log-odds that a memory chunk will be needed:

    Bi = ln(sum_j tj^(-d))

where tj is wall-clock seconds since the j-th access and d is the decay parameter (typically 0.5). This equation is the mathematically optimal Bayesian estimator of P(need | history) given the environmental regularity that information recurrence follows a power law in time (Anderson & Schooler, 1991).

The problem: VaultMind is a CLI tool. Between invocations, no cognitive processing occurs. A note accessed 30 wall-clock days ago might represent "1 session ago" for a monthly user. Standard ACT-R treats this note as heavily decayed (activation approximately ln(2592000^(-0.5)) = -7.4), making it nearly irretrievable. For a daily user, the same 30-day-old note is appropriately stale. For a monthly user, it was their most recent work.

This is not merely a tuning problem. Adjusting the decay parameter d does not solve it: lowering d slows decay for ALL notes uniformly, when the actual problem is that decay should be context-dependent — fast during active sessions, slow during inactivity.

### 2. Seven Approaches Analyzed

#### 2.1 Standard ACT-R (Wall-Clock Time)

Mechanism: Use the equation as-is with wall-clock timestamps.

Theoretical basis: Anderson & Schooler (1991) demonstrated that the power-law form matches the statistical structure of real-world information environments — newspaper headlines, library checkouts, email arrival patterns. The equation is justified as an optimal adaptation to these statistics.

Problem for VaultMind: These environments are "always on." Newspapers publish daily. Email arrives continuously. A CLI tool that sits idle for weeks breaks the stationarity assumption. The environmental statistics that justify the equation were measured over populations with relatively uniform activity, not individual users with bursty patterns.

Verdict: Correct in theory, penalizes infrequent users in practice.

#### 2.2 Frozen Time (Active Time Only)

Mechanism: Only count time during active sessions. Maintain a running counter of "active seconds."

Theoretical basis: None from cognitive science. Pure engineering heuristic.

Problems:
- Violates environmental regularity. A note about a bug accessed 30 wall-clock days ago may be stale because the bug was fixed — regardless of session count. The world changes during inactive periods.
- Pathological for returning users. After 6 months away, all notes would have activation levels from their last session — as if no time passed. This is clearly wrong.
- Incomparable across users. "100 active hours" means 100 days for a daily user and 2+ years for a weekly user.

Verdict: Theoretically unjustified. Avoid.

#### 2.3 Session Count as Time Unit

Mechanism: Replace tj with sj (number of sessions since j-th access).

Theoretical basis: Howard & Kahana's Temporal Context Model (2002) represents time as experienced events, not clock ticks. Sessions are coherent cognitive events; session count measures contextual distance.

Problems:
- Loses within-session temporal structure. Two accesses in the same session collapse to sj = 0.
- Non-uniform session density. A 5-minute lookup and a 3-hour research session contribute equally.
- Still violates environmental regularity — 10 sessions over 10 months is fundamentally different from 10 sessions over 10 days.

Verdict: Useful intuition but too coarse as the sole time measure.

#### 2.4 Compressed Idle Time (Recommended)

Mechanism: Transform the time variable so idle periods are compressed:

    tj_effective = active_time + gamma * idle_time

where gamma is between 0 and 1. During active sessions, time passes at rate 1. During idle periods, time passes at rate gamma.

Theoretical basis: Hybrid of Anderson's environmental regularity (wall-clock time matters) and session-based recommender models that treat inter-session gaps as producing reduced but non-zero state drift (Hidasi et al., 2016; Zhu et al., 2017). Analogous to biological sleep: time passes, some consolidation occurs, but at a reduced rate compared to waking cognition.

Properties:
- gamma = 1.0: pure wall-clock time (standard ACT-R)
- gamma = 0.0: pure frozen time
- gamma = 0.1-0.3: idle time counts but at reduced rate

Advantages:
- Preserves the power-law form, maintaining theoretical grounding
- Acknowledges that the world changes during inactivity, but at a slower effective rate
- Single tunable parameter
- Simple to implement: requires only session start/end timestamps

Verdict: Best fit for intermittent CLI tools. Recommended as primary approach.

#### 2.5 Dual-Strength Model (Complementary)

Mechanism: Implement Bjork & Bjork's (1992) dual-strength theory:
- Storage strength: monotonically increasing function of total accesses. Never decays.
- Retrieval strength: recency-weighted activation. Decays with time.

Final score combines both: score = w1 * retrieval_strength + w2 * storage_strength.

Theoretical basis: Bjork & Bjork's New Theory of Disuse is one of the most influential modern memory theories. The key insight: well-learned items (high storage strength) remain recoverable even when their retrieval strength is temporarily low. A note used 50 times over a month, then untouched for a year, is not the same as a note accessed once a year ago — even though both might have similar retrieval strength.

Implication for VaultMind: access_count serves as a proxy for storage strength. The existing schema already tracks this. Combined with retrieval strength from the activation equation, this prevents catastrophic loss of well-used notes after long hiatuses.

Verdict: Essential complement to any temporal decay model.

#### 2.6 Variable Decay Rate (Pavlik & Anderson)

Mechanism: The decay parameter d varies per access based on activation at the time of retrieval:

    dj = c * e^(Aj) + a

Items retrieved at high activation (massed practice) decay faster. Items retrieved at low activation (spaced practice) decay slower.

Theoretical basis: Pavlik & Anderson (2005, 2008) mechanized the spacing effect within ACT-R. Cross-session retrievals produce more durable memories than within-session retrievals because the note's activation is lower at the time of cross-session access.

Implication for VaultMind: A note accessed across 5 different sessions should have stronger activation than one accessed 5 times in a single session. This is already partially captured by the standard equation (the tj values are more spread out), but variable decay makes the effect explicit.

Verdict: Most principled long-term solution but requires more data to calibrate. Defer to v3.

#### 2.7 Three-Signal Scoring (Generative Agents)

Mechanism: Park et al. (2023) combined recency, importance, and relevance as three independent signals for memory retrieval in agent simulations. Recency used exponential (not power-law) decay.

Problem: Exponential decay does not match the environmental regularity findings. The power-law form is empirically superior (Wixted & Ebbesen, 1991). Also requires an "importance" score that VaultMind has no mechanism to assign.

Verdict: Good conceptual framework but inferior mathematical foundation. VaultMind's hybrid retrieval (semantic + keyword + graph) already captures relevance and importance more rigorously.

### 3. Expert Panel Discussion

Seven experts evaluate the proposed compressed-idle-time approach with dual-strength complement.

**Elena Vasquez (Cognitive Architecture):**
The compressed-idle-time approach is the right compromise. Anderson's rational analysis framework asks "what is P(need | history)?" — and the answer genuinely depends on whether the user was actively working during the gap. A gamma of 0.2 effectively says "the world changes at 20% of its normal rate while I'm not looking," which is a reasonable prior for a personal knowledge base whose topics evolve slowly.

My concern: gamma should not be a single global constant. Different types of notes decay at different rates. A note about a current project should decay faster during inactivity (the project moves on without you) than a note about a fundamental concept (spreading activation doesn't become less relevant over time). Consider type-specific gamma values in the future.

**Marcus Chen (Systems Engineering):**
Implementation is clean. You need a sessions table (session_id, started_at, ended_at) and an access_events table (note_id, session_id, accessed_at). Computing tj_effective requires partitioning the time between each access and now into active/idle segments, then applying gamma. This is O(sessions) per note per query — fast for a personal vault but could be precomputed and cached if it ever matters.

The dual-strength model is trivially cheap: access_count is already tracked. Just add it as a weighted term in context-pack scoring. No new queries needed.

**Diana Blackwell (Information Retrieval):**
I want to push back on one thing: you're applying activation to context-pack assembly, not to search ranking. This is the correct decision. Search should be query-driven (the user asked a specific question), not popularity-driven. But context-pack is assembling "relevant background" — and that's exactly where "what have I been working with recently" should matter.

However, consider the cold-start problem. A new vault has no access history. All notes have zero activation. Context-pack should still work well in this case. Make sure the activation score is a boost, not a gate — notes with no access history should still be included based on graph proximity, just not prioritized.

**Ravi Sharma (Cognitive Science):**
The Bjork & Bjork dual-strength complement is essential and I'm glad it's included. Here's why: the compressed-idle-time model, even with gamma = 0.2, will still significantly decay notes after a long hiatus. A note accessed 50 times during an intense project week, then untouched for 3 months, would have retrieval strength near zero. Without storage strength, it would be ranked below a note accessed once last week. That's wrong — the 50-access note represents deep engagement and should be preferentially surfaced when its topic neighborhood is activated.

Storage strength should use a logarithmic transform of access_count: storage = ln(1 + access_count). This prevents notes with 500 accesses from dominating notes with 50 — the marginal value of each access decreases, matching the psychological finding that overlearning has diminishing returns.

**Viktor Novak (Agent Architecture):**
From the agent perspective, this feature makes VaultMind adaptive without explicit configuration — exactly what an agent needs. The agent doesn't manage memory; it just uses the tool, and the tool learns from usage patterns. That's the Hebbian principle in action.

One practical point: the agent should not need to know about activation mechanics. The scoring should be transparent — if an agent asks "why was this note ranked higher," the answer should be available (debugging), but the agent's interface (search, ask, context-pack) should just return better results without any new flags or parameters.

**Yuki Nakamura (Applied ML):**
The gamma parameter is the weakest part of this design — it's a hyperparameter with no principled way to set it. Anderson's d = 0.5 is validated by decades of data. Gamma has no such validation. You could argue gamma = 0.2 "feels right," but you have no ground truth to calibrate against.

Suggestion: log all activation scores and retrieval outcomes. After enough data accumulates, you could empirically fit gamma by measuring whether high-activation notes were actually accessed more often in subsequent sessions. This is the same approach FSRS used for Anki — start with a reasonable default, collect data, refine. Don't pretend the initial value is principled when it's just a guess.

**Hans Hoffmann (Knowledge Management):**
I want to connect this back to the vault's own research on desirable difficulties and the spacing effect. The compressed-idle-time model naturally rewards spaced access patterns: notes accessed across many sessions (spread over time) accumulate more effective activation than notes crammed in a single session. This is not just a nice property — it's the fundamental finding from Pavlik & Anderson. The architecture should make spaced retrieval practice a natural consequence of normal usage, not something the user has to deliberately manage.

Also: save the gamma value and all activation parameters in the vault's config, not as hardcoded constants. The user explicitly said they want to tune this. Respect that.

### 4. Design Recommendation

Based on the analysis and expert discussion:

**Primary: Compressed Idle Time**
- Transform tj to tj_effective = active_time + gamma * idle_time
- Requires: sessions table with start/end timestamps, access_events table
- Gamma configurable via memory.activation_idle_rate (default 0.2)
- Decay parameter d configurable via memory.activation_decay (default 0.5)

**Complement: Dual-Strength Scoring**
- Retrieval strength: Bi = ln(sum tj_effective^(-d))
- Storage strength: Si = ln(1 + access_count_i)
- Combined score in context-pack: alpha * Bi + beta * Si (alpha, beta configurable or fixed)

**Integration point: Context-Pack assembly only**
- Search ranking is query-driven, unaffected
- Context-pack uses activation to prioritize which neighbor notes fill the token budget
- Cold-start safe: notes with no access history get activation = 0, still included by graph proximity

**Data requirements:**
- sessions table: (session_id TEXT, started_at TEXT, ended_at TEXT)
- access_events table: (note_id TEXT, session_id TEXT, accessed_at TEXT)
- Increment on: note get, memory recall, memory context-pack, ask (context items)

### 5. References

- Anderson, J.R. (1990). The Adaptive Character of Thought. Erlbaum. DOI: 10.4324/9780203771730
- Anderson, J.R. & Schooler, L.J. (1991). Reflections of the Environment in Memory. Psychological Science, 2(6), 396-408. DOI: 10.1111/j.1467-9280.1991.tb00174.x
- Bjork, R.A. & Bjork, E.L. (1992). A new theory of disuse. In Healy et al. (Eds.), From Learning Processes to Cognitive Processes, Vol. 2, 35-67. Erlbaum.
- Bliss, T.V.P. & Lomo, T. (1973). Long-lasting potentiation of synaptic transmission. Journal of Physiology, 232(2), 331-356. DOI: 10.1113/jphysiol.1973.sp010273
- Hebb, D.O. (1949). The Organization of Behavior. Wiley.
- Howard, M.W. & Kahana, M.J. (2002). A Distributed Representation of Temporal Context. Journal of Mathematical Psychology, 46(3), 269-299. DOI: 10.1006/jmps.2001.1388
- Koren, Y. (2010). Collaborative Filtering with Temporal Dynamics. Communications of the ACM, 53(4), 89-97. DOI: 10.1145/1721654.1721677
- Park, J.S. et al. (2023). Generative Agents: Interactive Simulacra of Human Behavior. UIST 2023.
- Pavlik, P.I. & Anderson, J.R. (2005). Practice and Forgetting Effects on Vocabulary Memory. Cognitive Science, 29(4), 559-586. DOI: 10.1207/s15516709cog0000_14
- Pavlik, P.I. & Anderson, J.R. (2008). Using a model to compute the optimal schedule of practice. J. Exp. Psych: Applied, 14(2), 101-117. DOI: 10.1037/1076-898X.14.2.101
- Wixted, J.T. & Ebbesen, E.B. (1991). On the form of forgetting. Psychological Science, 2(6), 409-415. DOI: 10.1111/j.1467-9280.1991.tb00175.x
