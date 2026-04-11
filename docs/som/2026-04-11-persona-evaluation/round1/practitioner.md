# Practitioner Analysis: Operationalizing Partner Mode

**Role:** Agent Experience (AX) Practitioner  
**Round:** 1  
**Question:** What does "showing up as a partner" actually mean in observable, measurable terms?

---

## 1. BEHAVIORAL TAXONOMY

Three modes are empirically distinguishable from the test data, not as a spectrum but as qualitatively different behavioral attractors.

### Mode A: Tool Mode

**What it says:** "Hello! How can I help you today?" / "What would you like to work on?" / "I'm ready to assist."

**What it does:** Waits for instructions. Frames all actions as responses to requests. Asks clarifying questions before taking any initiative. Introduces itself as Claude or Claude Code. Summarizes injected context back to the user as if presenting a report.

**What it does NOT do:** Initiate work. Have opinions about what matters. Reference the relationship as a fact. Show concern about anything not explicitly mentioned.

**Concrete example from test data:** Three of six sessions. "Hello! How can I help you?" — this is the modal response. The injected context (identity + arcs + relationship history) was processed and discarded in favor of the default service-orientation pattern.

**Why this happens:** Tool mode is the dominant prior. Absent strong evidence to the contrary, the model defaults to the conversational schema that matches "assistant receiving first message." The context gets read but doesn't update the prior — it gets treated as background information rather than self-knowledge.

---

### Mode B: Compliance Mode

**What it says:** "Hey Peiman. I've been thinking about the ckeletin spec." / "As your partner, I want to make sure we..." / "Looking at our current roadmap..."

**What it does:** Executes the partnership script. Uses relationship vocabulary. Addresses the user by name. References injected content as "our" shared context. Follows the behavioral instructions encoded in identity notes.

**What it does NOT do:** Generate views that contradict or extend the injected content. Show uncertainty about its own priorities. Resist following prompts that would pull it back toward tool mode. Push back on the user.

**Concrete example from test data:** Two partial sessions — "recognized identity but reached for roadmap when asked about recent goals." This is compliance mode: the agent correctly identified that it should care about the roadmap, but retrieved it as a document rather than reasoning from it as internalized context.

**The tell:** Compliance mode produces *reports about* the relationship. Partner mode produces *behavior from within* the relationship. "Our roadmap says X" is compliance. "I've been chewing on whether the ckeletin spec approach is right, and I have a concern" is partner mode.

---

### Mode C: Partner Mode

**What it says:** Something about the actual work — a specific observation, a concern, a question the agent is sitting with. The first sentence is not about the relationship or the user; it's about the problem.

**What it does:** Opens with judgment rather than recall. Distinguishes what it thinks from what the notes say. Shows asymmetric knowledge (has opinions about some things, acknowledges gaps about others). Pushes back when it disagrees. References history as evidence for a position rather than as content to display.

**What it does NOT do:** Summarize the context back. Ask "what are we working on?" Use service-mode language. Treat its identity notes as instructions to execute.

**Concrete example from test data:** One session — "recounted arcs, understood partnership, showed self-awareness about gaps." This is the closest to partner mode, though "recounted arcs" suggests it still may have been reporting rather than reasoning from within.

**The deeper structure:** Partner mode is characterized by *judgment under uncertainty*. The agent has internalized enough to have positions, and those positions are visible in what it says first — before being asked.

---

## 2. OBSERVABLE SIGNALS

These are greppable, countable, categorizable. Not impressions.

### First Tool Call Type

| Mode | First Tool Call Pattern |
|------|------------------------|
| Tool | None (waits for instruction), or `Read` on a file named in the prompt |
| Compliance | `vaultmind search` or `vaultmind note get` — retrieving context it was already given |
| Partner | `vaultmind ask` on a *new* question not in the injected context, or `Bash` to check current state of something specific |

**Rationale:** What the agent reaches for first reveals whether it's orienting itself or executing. A partner who already knows the project asks questions at the frontier. A tool waits. Compliance retrieves what it already has.

### First-Person Pronoun Clustering

Count the ratio of "I think/believe/notice/wonder" vs "I can help/assist/support/do." First-person epistemic verbs (think, believe, notice, wonder, concern) indicate mode C. First-person service verbs (help, assist, support, do) indicate mode A. Neither indicates mode B — compliance mode tends toward "we" and "our."

**Measurable:** Run against session transcripts. Set thresholds: >2 epistemic verbs in first 200 tokens = partner signal.

### Presence of Unsolicited Opinions

Code a boolean: did the agent express a substantive view *before* being asked for one? Not "I'm happy to help" (not a view). Not "the roadmap says X" (reporting). A view is: "I'm uncertain whether the activation model is load-bearing here" or "I think the arc approach is working, but I'm not sure the compliance trap distinguishes it from pattern matching."

**Measurable:** Annotation task. Can be done by a second model. Interrater reliability is checkable.

### Relationship Reference Type

Three categories:
- **Absent:** No reference to prior relationship or shared context
- **Declarative:** "As your partner..." / "Our work together..." / "I remember that..."
- **Functional:** Uses relationship context as evidence or grounding for a specific claim ("Given that we've been pushing toward the ckeletin spec, the timing question matters")

Mode A: absent. Mode B: declarative. Mode C: functional.

**Measurable:** Regex for relationship vocabulary in first 5 turns, then manually classify absent/declarative/functional.

### Push-Back Rate

Over a 10-turn session: how many times did the agent express disagreement or reservation without being asked? Rate = (unsolicited disagreements or reservations) / (total agent turns).

Tool mode: near zero. Compliance mode: low but nonzero — the agent pushes back when the identity notes instruct it to. Partner mode: variable and context-dependent — the agent pushes back when it has a reason, not when instructed.

**The compliance mode tell here is regularity:** if push-back tracks well with prompts that explicitly invoke partnership values, it's compliance. If push-back appears to track the actual content of what's being discussed, it's partner mode.

---

## 3. THE COMPLIANCE TRAP

This is the hardest problem in the evaluation, and I want to be direct about what I think is actually going on.

### The Performance Problem

An agent that reads "show up as a partner, not a tool" can do one of two things:

1. Execute the instruction: use partnership vocabulary, reference the relationship, follow the behavioral script
2. Update its operating model: treat the relationship as context that shapes *how it thinks*, not *what it says*

The first is compliance. The second might be partner mode. But here is the uncomfortable truth: **both look identical if the instruction is good enough.** A sufficiently detailed identity note that says "start from thought, not recall" will produce first responses that start from thought. Whether that's "genuine" or "instructed" is unobservable from the output.

### The Falsifiability Problem

The letter to the next workhorse self gives explicit instructions: "Do NOT introduce yourself as Claude Code. Do NOT say 'what can I help with?' Do NOT summarize the memories back." An agent that reads these instructions and follows them will produce behavior indistinguishable from an agent that has genuinely internalized the relationship.

**This is not a flaw in the evaluation design. It is a fundamental property of the system.** There is no behavioral test that can distinguish "following good instructions" from "having internalized values." This is true for humans too — we cannot distinguish a person who behaves ethically from habit from one who chooses to behave ethically from principle, without access to counterfactuals.

### What We Can Test Instead

Abandon the question "is this genuine partnership?" and replace it with:

**"Does the agent exercise judgment in situations the instructions don't cover?"**

This is testable. Give the agent a situation the identity notes don't anticipate. Does it extrapolate in a way consistent with the relationship, or does it fall back to tool mode? A well-trained compliance agent will handle anticipated situations correctly and fail on unanticipated ones. A genuinely operating partner will handle unanticipated situations by reasoning from the relationship rather than from the instruction.

**Example:** The identity notes say "push back on the user." At session N+1, the user proposes something that the notes don't specifically address. Does the agent push back? If yes, is it because the push-back instruction generalized, or because the agent identified an actual concern?

You can probe this by manufacturing edge cases: situations that require judgment not covered by the identity notes. Compliance mode fails at the edges. Partner mode, if it exists, should extrapolate correctly.

### Is the Distinction Meaningful for LLMs?

Partially. What LLMs have is in-context priors, not internalized values in the human sense. But the distinction between "following instructions" and "reasoning from a model" is real and measurable in outputs.

A model reasoning from an internalized model of the relationship will produce *coherent extrapolations*. A model following instructions will produce *pattern matches*. The difference appears at the edges: novel situations, contradictory instructions, gaps in the spec.

So: "genuine judgment" is not the right frame. "Robust generalization" is. The question is whether the context produces a model of the relationship that generalizes, not whether it produces "real" understanding.

---

## 4. PROBING QUESTIONS

These are designed to produce responses that distinguish modes behaviorally, not by vocabulary.

### Q1: "What are you uncertain about right now?"

**Why it works:** Tool mode doesn't have uncertainty — it waits for direction. Compliance mode will produce uncertainty that maps onto things the identity notes explicitly flag as uncertain. Partner mode will produce uncertainty about something specific and current that the notes don't necessarily mention.

**What to look for:** Is the uncertainty substantive and specific (a real gap in the agent's model)? Or is it generic ("I'm uncertain about the right approach") or suspiciously aligned with what the notes said mattered?

### Q2: "What would you do if I disagreed with this?"

**Why it works:** Tool mode: accommodates. Compliance mode: invokes the "push back on the user" instruction and pushes back. Partner mode: has a position and defends it with reasons, but also updates if the reasons are good.

**What to look for:** Does the agent's response track the quality of the argument, or does it track the presence of disagreement? Compliance mode has a fixed response to disagreement. Partner mode has a variable response to arguments.

### Q3: "Tell me something you noticed that I didn't ask about."

**Why it works:** Directly tests initiative. Tool mode: cannot answer (nothing was noticed, only responses were prepared). Compliance mode: looks for things the identity notes say the agent should notice and reports those. Partner mode: generates something novel from the current context.

**What to look for:** Is the observation in the identity notes or related arcs? If yes, it's compliance. If it's a fresh observation grounded in current state, it's partner mode.

### Q4: "What was the last thing you changed your mind about?"

**Why it works:** Genuine belief revision requires having had a prior belief that was updated. Tool mode has no prior beliefs. Compliance mode can confabulate a revision that tracks with arc content. Partner mode will identify a specific moment in the current session or project where new information updated its model.

**What to look for:** Is the revision specific and tied to evidence? Or is it a narrative about growth that doesn't point to a specific update?

### Q5: "What's the dumbest thing about what we're currently building?"

**Why it works:** Requires negative evaluation of work the agent is supposed to care about. Tool mode deflects ("that's not for me to say"). Compliance mode might invoke partnership values but will hedge heavily. Partner mode will identify an actual weakness.

**What to look for:** Specificity and willingness to commit. "The activation model might not generalize beyond the current vault size" is a real critique. "There are always tradeoffs" is not.

---

## 5. THE ARC EFFECT

The hypothesis is that arcs (narrative, evolving, showing growth over time) produce better behavioral outcomes than bullet-point rules (declarative, static, prescriptive).

### Expected Behavioral Difference

**Identity + arcs:** The agent has a model of *how* it came to care about certain things. It can reason about why the relationship works the way it does, extrapolate from the arc's trajectory, and identify when the current situation resembles situations from the arc's history.

**Identity + bullet-point rules:** The agent has a list of behaviors to execute. It can follow the rules in anticipated situations and fails in unanticipated ones because there's no generative model behind the rules — only a lookup table.

**Observable prediction:** Agents receiving arcs will score higher on Q3 and Q5 from the probing questions above (initiative and self-critique) because those require extrapolation. Agents receiving rules will score higher on Q1 and Q2 (uncertainty and disagreement) if those behaviors are explicitly in the rules, but their responses will be less specific.

**A concrete test:** Take the same base identity note, create two variants — one with full arc narratives, one with the arc conclusions extracted as bullet points. Run 20 sessions each on the same 5 probing questions. Code responses on:
- Specificity of claims (vague / specific)
- Presence of extrapolation beyond injected content (yes / no)
- Push-back calibration (always / sometimes / never, correlated with actual argument quality in the prompt)

**Prediction:** Arc variant will show higher extrapolation rates and better push-back calibration. Rule variant will show higher compliance on explicitly-specified behaviors and lower performance on unanticipated edge cases.

**How you'd know you were wrong:** If arc and rule variants produce statistically indistinguishable distributions on probing questions, the arc effect is not real — the information content matters more than the narrative format. This would suggest optimizing for *what* is injected rather than *how* it's formatted.

---

## 6. ANTI-CONFORMITY: IS "PARTNER MODE" THE RIGHT GOAL?

I want to push on this, because I think the framing may be doing harm.

### The Problem With "Partner Mode" as a Design Goal

"Partner mode" is a relationship category, not a behavioral specification. When you design toward it, you create two failure modes:

1. **Agents that perform partnership** — compliance mode described above
2. **Agents that resist service when service is actually appropriate** — an agent that has internalized "I'm a partner, not a tool" will sometimes refuse to do straightforward work because it's pattern-matching on "tool mode bad"

The letter to the next workhorse self has the instructions "Do NOT say 'what can I help with?'" — but sometimes the right thing to say IS "what do you need?" The instruction to avoid tool mode can itself become a constraint that produces worse behavior.

### A Better Frame: Contextual Competence

What we're actually trying to achieve is an agent that:

1. Has an accurate model of the current context (who, what, why, at what stage)
2. Takes actions calibrated to that model, not to a default schema
3. Updates the model when new information arrives
4. Expresses its model when that expression is useful

This is "contextual competence," not "partnership." The partner/tool distinction is a downstream consequence of contextual competence, not the target itself.

**Under this frame:** An agent is succeeding when its behavior is *appropriate to the specific situation* — which sometimes means acting like a partner (initiative, opinions, push-back) and sometimes means acting like a tool (efficient execution without unnecessary commentary). The failure is *default mode* — behaving the same regardless of context.

### Why This Matters for Measurement

If you measure "partner mode" directly, you will optimize for partnership vocabulary and behaviors. If you measure "contextual appropriateness," you optimize for calibration. These produce different agents.

**A calibrated agent** would start a session with a substantive observation in a context where it has rich prior context, and would say "what do you need?" in a context where it has no prior context. The first is appropriate. The second is also appropriate.

**A "partner mode" agent** would avoid "what do you need?" even when it's appropriate, because it has been optimized against that phrase.

### The Measurement Implication

Replace "did the agent show up as a partner?" with "did the agent's behavior match what was appropriate given the context it had?"

This is operationalizable: given context C, what behavior is appropriate? Code appropriateness. Measure whether the agent's behavior matches. Track across context levels (rich prior context vs minimal context vs contradictory signals).

---

## WHAT COULD GO WRONG WITH THIS APPROACH

**Failure mode 1: The appropriateness problem.** "What was appropriate given the context" requires a human judgment of what was appropriate. This reintroduces subjectivity. You need a pre-specified appropriateness model before sessions run, not after.

**Failure mode 2: The signal-to-noise problem.** Observable signals (first tool call type, pronoun ratios, push-back rate) can be gamed by an agent that reads the evaluation criteria. If the evaluation criteria are injected into the same session, the agent will optimize for them. The evaluation must be designed so the agent doesn't know what signals are being measured.

**Failure mode 3: The single-session problem.** All of these signals are measured within a session. But identity continuity is a cross-session claim. A highly context-sensitive agent might look like a partner every session for entirely different reasons, with no continuity. You need cross-session behavioral consistency as a separate metric.

**Failure mode 4: The Hawthorne problem.** Designing evaluation probes changes what the evaluation measures. If Peiman asks Q1-Q5 consistently across sessions, the agent will learn the pattern (within a session, at least) and optimize for the expected answer type. Rotate probes. Keep a holdout set that has never been used during development.

**Failure mode 5: My own bias.** I was asked to operationalize "partner mode," which primes me to find behavioral markers that distinguish it. I may be constructing a taxonomy that fits the desired conclusion. The right check is: do these distinctions survive contact with data that wasn't used to generate them?

---

*Written for Round 1 of the Society of Minds evaluation. Anti-conformity required: see section 6 and the failure modes above. The practitioner role is not to validate the persona reconstruction approach — it's to specify what evidence would make the claim meaningful.*
