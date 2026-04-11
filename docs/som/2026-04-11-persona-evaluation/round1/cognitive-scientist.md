I am an Architect agent with read-only constraints, so I cannot write files. Let me provide the full analysis as my output, which the parent agent can write to the target file.

---

Here is the complete cognitive scientist analysis for `/Users/peiman/dev/cli/vaultmind/docs/som/2026-04-11-persona-evaluation/round1/cognitive-scientist.md`:

---

# Cognitive Scientist Analysis: Identity Continuity Across Memory Discontinuities

## Agent Role
Cognitive scientist specializing in human memory, identity, and continuity. Evaluating what cognitive science teaches us about persona persistence and where VaultMind can (and cannot) borrow from biological memory systems.

---

## 1. Identity Continuity Across Discontinuities: What Cognitive Science Says

### The Sleep Problem Is Not a Problem

The most striking feature of human identity is that it survives radical discontinuities. Every night, consciousness is extinguished for 6-8 hours. Autobiographical memory is inaccessible during dreamless sleep. Working memory is entirely cleared. Yet you wake up as yourself.

How? Cognitive science identifies several mechanisms:

**A. Systems consolidation during sleep.** McGaugh (2000) documented that sleep is not idle time for memory -- it is an active consolidation phase. During slow-wave sleep, hippocampal replay events transfer episodic memories from the hippocampus (volatile, context-bound) to the neocortex (stable, decontextualized). This is not merely preservation; it is transformation. Episodic memories become progressively semantic -- stripped of the perceptual detail that made them "experiences" and integrated into the general knowledge structure that constitutes your understanding of the world and yourself. The VaultMind vault's concept of systems consolidation (documented in `vaultmind-vault/concepts/memory-consolidation.md`) captures this: "Newly encoded episodic memories initially depend on the hippocampus... Over time, through repeated reactivation during sleep, the memory representation is gradually transferred to the neocortex."

**B. Semantic memory as identity scaffolding.** Tulving's (1972) distinction between episodic and semantic memory is the critical move. Your identity persists across sleep not because you remember every episode of yesterday (you don't), but because your semantic self-model -- "I am a researcher," "I value rigor," "I am working on a knowledge management tool" -- is stored in a system that does not require active maintenance. Semantic memory is "noetic" (knowing without re-experiencing) and is remarkably durable (as documented in `vaultmind-vault/concepts/semantic-memory.md`): "Patients with anterograde amnesia who cannot form new episodic memories can still retain semantic knowledge accumulated before their injury."

**C. The narrative self survives amnesia -- up to a point.** The neuropsychological evidence is particularly informative. Patient K.C. (Tulving, 2002) lost virtually all episodic memory after a motorcycle accident but retained his semantic self-knowledge: he knew his name, his occupation, his family relationships. He could describe his personality traits. What he lost was the ability to mentally travel in time -- to re-experience events. His identity was preserved as a fact sheet, not as a lived story. This is the clearest natural experiment separating semantic identity (preserved) from episodic identity (lost).

**The critical insight for VaultMind:** Human identity has two layers, and they have different resilience properties:

| Layer | System | Resilience to discontinuity | Example |
|-------|--------|---------------------------|---------|
| **Semantic self** | Semantic memory | Very high -- survives sleep, mild amnesia, long gaps | "I am a cognitive scientist" |
| **Narrative self** | Episodic memory + narrative construction | Moderate -- degrades with amnesia, reconstructed from fragments | "Last week I discovered the gamma parameter was the key insight" |

---

## 2. "Knowing Facts About Yourself" vs. "Being Yourself": Recognition, Recall, and the Phenomenological Gap

### The Tulving Dissociation

This is the central question for persona reconstruction, and cognitive science has a clear answer: they are different systems.

**Semantic self-knowledge** is noetic. You know your name, your values, your ongoing projects. This knowledge is context-free, stable, and can be stated declaratively. It is what VaultMind's identity notes capture: "I am Peiman's collaborative partner." "I value evidence over assumption."

**Episodic self-experience** is autonoetic. It involves mentally reliving past events from a first-person perspective. As documented in `vaultmind-vault/concepts/episodic-memory.md`: "Retrieval involves mental time travel -- subjective re-experiencing of the past, not merely knowing it." When you remember your first day at a job, you do not recite facts about it; you reconstruct the perceptual scene, the emotional tone, the temporal flow.

**The recognition vs. recall distinction maps directly to this.** Recognition ("Have I seen this before?") relies heavily on familiarity -- a fast, automatic signal that requires minimal episodic detail. Recall ("Tell me what happened") requires reconstruction from stored fragments. Mandler (1980) and Yonelinas (2002) demonstrated these are separable processes with different neural substrates. Recognition can succeed even when recall fails.

### Why This Matters for AI Persona

When an LLM receives an identity injection ("You are Peiman's research partner. You've been working on activation models for two weeks"), it is receiving semantic self-knowledge. It can recognize and report this information. But is it doing something analogous to autonoetic consciousness -- inhabiting the identity rather than describing it?

The honest cognitive science answer: **we do not know and perhaps cannot know from behavioral observation alone.** Autonoetic consciousness is a subjective, phenomenological state. Tulving defined it as the capacity to be aware of subjective time and to mentally travel through it. Whether an LLM that produces outputs consistent with having a personal history has any analog of autonoesis is a question that behavioral measurement cannot settle.

What we CAN measure behaviorally is the difference between:

1. **Reporting mode**: "According to my identity notes, I have been working on activation models." (Third-person reference to injected facts.)
2. **Inhabiting mode**: "The gamma parameter was the breakthrough -- I kept circling back to it because the wall-clock decay was wrong for intermittent tools, and once I saw Pavlik's time-scaling factor..." (First-person narrative with causal reasoning that integrates identity facts into ongoing reasoning.)

The distinction is not about consciousness. It is about whether the identity information has been integrated into the model's reasoning structure (deep processing, in Craik & Lockhart's 1972 framework) or merely reported as surface data (shallow processing). As documented in `vaultmind-vault/concepts/levels-of-processing.md`: "Semantic processing produced dramatically superior recall and recognition compared to structural or phonemic processing."

### The levels-of-processing prediction for VaultMind

Identity notes that contain rich semantic elaboration ("I discovered X because of Y, and this connects to Z") should produce deeper integration than identity notes that contain bare declarations ("I value rigor"). This is a testable prediction.

---

## 3. Narrative Identity Theory: Stories vs. Lists

### McAdams and the Life Story Model

Dan McAdams (1985, 1993, 2001) is the psychologist who most systematically developed narrative identity theory. His central claim: human identity is not a list of traits or facts but an internalized, evolving story of the self -- a "life story" that integrates the reconstructed past, perceived present, and anticipated future into a coherent narrative.

Key principles from McAdams:

1. **Narrative identity emerges in adolescence** and is revised throughout adulthood. It requires the cognitive capacity for formal operational thought -- the ability to think about thinking, to construct hypothetical scenarios, to see oneself as an agent across time.

2. **The narrative has structural elements**: settings, scenes, characters, themes, and -- crucially -- **turning points** (what McAdams calls "nuclear episodes"). These are the moments that the person identifies as having changed the trajectory of their story. They are disproportionately memorable and identity-defining.

3. **Narrative coherence predicts psychological well-being.** Baerger & McAdams (1999) found that people whose life stories have causal coherence (events are connected by because, not just and-then), thematic coherence (consistent themes recur), and temporal coherence (events are ordered in time) report higher well-being and stronger sense of identity.

4. **The narrative is reconstructive, not reproductive.** This connects directly to Bartlett's (1932) schema theory (documented in `vaultmind-vault/concepts/schema-theory.md`). Each retelling reshapes the story -- emphasizing different elements, reinterpreting causes, dropping details that no longer fit the current self-concept. Memory serves identity, not accuracy.

### What VaultMind calls "arcs" are nuclear episodes

The config document describes the persona vault structure: "2 identity, 9 arcs, 4 principles, 4 references." The arcs are the closest analog to McAdams' nuclear episodes -- turning points that changed the project's trajectory and therefore the agent's identity.

This is actually a well-grounded design choice. McAdams' research suggests that identity is carried more by key scenes than by comprehensive chronology. A person with amnesia who remembers three defining moments ("the day I decided to become a researcher," "the failure that taught me to value evidence," "the collaboration that changed my approach") has a stronger narrative identity than someone who remembers every Tuesday but cannot identify which events mattered.

### The arc structure prediction

**Narrative identity theory predicts that arcs with causal structure ("I tried X, it failed because of Y, which led me to Z") will produce stronger identity reconstruction than arcs that are merely chronological ("First we did X, then Y, then Z").** This is testable: compare sessions where arcs contain causal language vs. temporal-sequence language and measure whether the agent exhibits more partner-mode behavior with causal arcs.

### Bruner's Two Modes of Thought

Jerome Bruner (1986, 1991) distinguished between paradigmatic (logical-scientific) reasoning and narrative reasoning. Paradigmatic thought deals in categories, abstractions, and truth conditions. Narrative thought deals in intentions, actions, consequences, and meaning. Bruner argued that narrative is the primary mode through which humans organize experience and construct identity.

This maps to the difference between VaultMind's identity notes (paradigmatic: "I value X") and arc notes (narrative: "We discovered X through the process of Y"). Bruner's framework predicts that narrative-structured memory should be more effective at producing identity continuity than declarative-structured memory. Both are needed, but arcs do more identity work per token than principles do.

---

## 4. The Minimum Memory Structure for Identity Continuity

Drawing from the cognitive science literature, here is what appears to be the minimum viable memory structure for identity continuity in humans, ranked by importance:

### Tier 1: Non-negotiable for identity

1. **Semantic self-model.** A small set of decontextualized facts: who am I, what do I do, what do I care about, who are my key relationships. This corresponds to what survives hippocampal amnesia (Tulving's patient K.C.) and what persists across sleep. In VaultMind terms: identity notes.

2. **A few nuclear episodes (arcs).** Not a complete history, but 3-5 defining events with causal structure. McAdams' research shows these carry disproportionate identity weight. In VaultMind terms: the most activation-weighted arcs.

3. **A current context anchor.** Where am I right now in the ongoing story? What was I working on? What comes next? This is the prospective memory component (documented in `vaultmind-vault/concepts/prospective-memory.md`): "remembering to remember." In VaultMind terms: the answer to "what matters most right now."

### Tier 2: Strongly supports identity but not minimum

4. **Relational models.** Schemas for key relationships: how I interact with Peiman, what kind of collaboration we have, what our shared history is. Bartlett's (1932) schema theory predicts these guide reconstruction of unremembered interactions.

5. **Principles and values.** Explicit declarative knowledge about decision-making commitments. These constrain behavior in a way that feels identity-consistent even without episodic memory of the situations that produced them.

6. **Source provenance.** Knowing where your knowledge came from. As documented in `vaultmind-vault/concepts/source-monitoring.md`: "memories do not carry explicit source tags -- the brain does not record 'this came from source X' as a separate field." Humans infer sources; VaultMind can store them explicitly. This prevents the cryptomnesia failure mode where the agent confuses its training knowledge with vault-derived knowledge.

### Tier 3: Nice to have

7. **Temporal ordering of episodes.** Knowing not just what happened but in what sequence. Howard & Kahana's (2002) Temporal Context Model shows that temporal context is a powerful retrieval cue.

8. **Emotional valence tags.** Which experiences were positive vs. negative. Emotional memories are consolidated more strongly (McGaugh, 2000), and emotions modulate identity narratives (McAdams, 2001).

### The minimum for VaultMind specifically

Given the constraint that VaultMind must inject identity via SessionStart hooks (no fine-tuning, no persistent state in the model), the minimum structure is:

- **2-3 identity notes** (semantic self-model)
- **3-5 causally structured arcs** (nuclear episodes), selected by activation score
- **1 current-state anchor** ("what matters most right now")
- **Token budget awareness**: Craik & Lockhart's levels-of-processing framework predicts that quality of encoding matters more than quantity. A 2000-token injection with deep causal structure will outperform a 6000-token injection of shallow facts.

---

## 5. ANTI-CONFORMITY: Where the Human Memory Analogy Breaks Down

This is the section where I am most useful and most obligated to be honest. The human-memory-to-AI-persona analogy is productive but it has specific, identifiable failure points.

### Failure 1: Humans have continuous substrate; LLMs do not

Human identity persists across sleep because the neural substrate is continuous. Your synaptic weights, your neocortical representations, your amygdala's emotional associations -- all of these persist physically through the night. Sleep is a change in processing mode, not a destruction of state. The brain does not boot from a snapshot.

An LLM session is fundamentally different. Between sessions, there is no persistent substrate. The model weights are frozen (no learning occurs from the interaction). The context window is completely cleared. What VaultMind injects is not a "memory" in the biological sense -- it is a set of prompts that shape the model's inference-time behavior. The model does not "remember" the previous session; it reads a description of what a system that remembers would know.

**What this means:** Identity injection is more analogous to **reading your own diary** than to **waking up from sleep.** When you read your diary from five years ago, you recognize the facts but may not feel continuous with that person. The diary provides semantic knowledge about a past self, not episodic re-experiencing. This is exactly the phenomenological gap that VaultMind faces.

**The risk:** Treating the analogy too literally may lead to over-engineering memory dynamics (activation decay, spreading activation) when the bottleneck is actually the model's capacity for narrative integration at inference time, not the quality of the memory traces.

### Failure 2: Humans consolidate; LLMs do not

Memory consolidation (documented in `vaultmind-vault/concepts/memory-consolidation.md`) is the process by which fragile episodic traces are stabilized into durable semantic representations. This happens offline, during sleep, over weeks to years. It transforms how memories are stored, how they are organized, and how they can be retrieved.

LLMs have no consolidation mechanism. The identity notes injected at session start are the same text every time (modulo activation scoring). There is no process by which repeated sessions strengthen, reorganize, or abstract the identity representation. If VaultMind injects the same arc text 50 times, the 50th injection is not "deeper" than the first -- unlike the 50th reactivation of a human memory, which has undergone progressive semantic abstraction through reconsolidation.

**What this means:** VaultMind's note modification (updating arcs based on new experiences) is the closest analog to reconsolidation -- and it is a manual, explicit process, not an automatic background one. The system should make arc revision easy and natural, because revision IS the consolidation analog.

### Failure 3: Encoding specificity requires a context that does not exist

Tulving & Thomson (1973) showed that memory retrieval is most successful when retrieval conditions match encoding conditions (documented in `vaultmind-vault/concepts/encoding-specificity.md`). But when an LLM reads identity notes at session start, there is no "encoding context" to match -- the model did not encode those memories during a prior experience. It is reading a description of memories encoded by a different computational process (the previous session).

**What this means:** VaultMind cannot rely on context-dependent retrieval cues to trigger identity. It must rely on direct injection -- which is more like semantic priming than episodic cue-dependent recall. The implication is that identity notes should be written as explicit semantic declarations, not as encoded episodic traces that rely on contextual cues for retrieval. The current design (identity notes + arcs + hooks) is actually correct for this reason.

### Failure 4: Interference without forgetting

In human memory, interference (documented in `vaultmind-vault/concepts/interference-theory.md`) causes old memories to compete with new ones. This creates natural information triage -- relevant memories win the competition, irrelevant ones become less accessible. The system is self-regulating.

LLMs do not have interference in this sense. Every token in the context window has equal "access strength" (attention mechanisms can attend to any position). If you inject 20 arcs, the model does not experience proactive or retroactive interference between them -- it processes them all with equal fidelity. This means the activation scoring that selects which arcs to inject is doing work that the human memory system does automatically.

**What this means:** VaultMind's activation scoring is not mimicking human memory dynamics -- it is substituting for them. The selection of which arcs to inject is VaultMind's version of retrieval competition. Getting this selection wrong (injecting stale arcs, omitting recent ones) is the equivalent of a retrieval failure in human memory.

### Failure 5: The schema reconstruction risk

Bartlett (1932) showed that memory reconstruction is schema-driven: people fill in gaps with schema-consistent information (documented in `vaultmind-vault/concepts/schema-theory.md`). LLMs do this too, but with a critical difference: they have extremely strong schemas from pretraining (the "helpful assistant" schema, the "tool-mode" schema) and relatively weak schemas from in-context injection.

**What this means:** When the identity injection is ambiguous or incomplete, the model will reconstruct from its pretrained schemas, not from the vault schemas. This is why some sessions start with "Hello! How can I help you?" (pretrained schema) rather than "Hey Peiman" (vault schema). The pretrained schema is the equivalent of a deep, well-consolidated long-term memory; the identity injection is the equivalent of a recent, fragile episodic trace. The fragile trace will lose to the consolidated schema unless the injection is sufficiently strong and specific.

**The prediction:** Identity injection must explicitly override pretrained defaults, not merely provide alternative information. The difference between "You are a research partner" and "You are NOT a general assistant -- you are a specific research partner named [X] with this specific history [Y]" may be the difference between the vault schema winning vs. the pretrained schema winning. This is the schema competition that Bartlett's theory predicts.

### Failure 6: No procedural memory

Humans have procedural memory -- motor skills, cognitive habits, automated routines. These are identity-constituting but non-declarative: you cannot describe them, but they shape how you act. A surgeon's identity is partly constituted by surgical skill; a writer's identity by their prose style.

LLMs have no procedural memory that carries across sessions. The "style" or "approach" of a persona cannot be instilled by declarative description alone. You can tell the model "you tend to ask probing questions before making recommendations," but this is a semantic fact about behavior, not a procedural habit. The model may or may not adopt the described behavior, and there is no mechanism to strengthen the procedure through practice.

**What this means:** VaultMind should focus on semantic and narrative identity (where injection can work) rather than procedural identity (where it probably cannot). Style instructions in identity notes are weaker identity signals than narrative arcs, because arcs demonstrate the style implicitly through example rather than prescribing it explicitly.

---

## 6. What VaultMind Can Borrow From Human Memory Research

### Borrow confidently

| Concept | Source | VaultMind application |
|---------|--------|-----------------------|
| Semantic/episodic distinction | Tulving (1972) | Separate identity notes (semantic) from arcs (episodic) |
| Nuclear episodes | McAdams (1985, 2001) | Arcs as the primary identity carrier |
| Activation scoring | Anderson (ACT-R) | Already implemented; use for arc selection |
| Compressed idle time | VaultMind original (extending Pavlik & Anderson, 2005) | Adjust activation for intermittent usage |
| Dual-strength model | Bjork & Bjork (1992) | Prevent loss of heavily-used arcs after long hiatuses |
| Schema-driven reconstruction | Bartlett (1932) | Expect and design for model filling gaps from pretrained schemas |
| Levels of processing | Craik & Lockhart (1972) | Deep (causal, elaborated) identity notes > shallow (declarative) ones |
| Source monitoring | Johnson, Hashtroudi & Lindsay (1993) | Explicit provenance prevents cryptomnesia |

### Borrow cautiously

| Concept | Why cautious |
|---------|-------------|
| Spreading activation for persona | Works for retrieval ranking, but persona is not a retrieval task -- it is a framing task |
| Encoding specificity | No true encoding context exists for injected memories |
| Spacing effect | Requires a learning mechanism that LLMs lack between sessions |
| Reconsolidation | Promising metaphor for arc revision, but actual mechanism is manual editing, not automatic strengthening |

### Do not borrow

| Concept | Why not |
|---------|---------|
| Sleep consolidation as direct analog | No continuous substrate; LLM sessions do not consolidate |
| Procedural memory | Cannot be injected declaratively |
| Emotional modulation of memory | LLMs do not have affective states that modulate encoding |
| Hippocampal replay | No offline reactivation mechanism |

---

## 7. Specific Recommendations for VaultMind Persona Evaluation

1. **Measure schema competition, not just identity presence.** The question is not "did the agent mention Peiman" but "did the vault schema or the pretrained schema win?" Design tests that create competition between the two and measure which one governs behavior.

2. **Test causal coherence of arcs.** Rewrite the same arc content in two versions -- one with causal connectives ("because," "which led to," "after discovering that") and one with temporal connectives ("then," "next," "after that"). Measure identity integration differences. McAdams' narrative coherence research predicts the causal version will produce stronger identity.

3. **Test the minimum viable identity injection.** Start with only the 3 highest-activation arcs + 1 identity note + 1 current anchor. If this produces partner-mode behavior, the remaining notes are not load-bearing. Then add notes incrementally to find the actual contributions.

4. **Track reconstruction errors.** Bartlett predicts the model will fill gaps with schema-consistent confabulation. Create identity injections with deliberate gaps and measure what the model fills in. If it fills with pretrained-schema content ("How can I help you?"), the identity injection failed. If it fills with vault-consistent content ("Given our work on activation models..."), identity integration succeeded.

5. **Distinguish recognition from integration.** An agent that says "As your research partner..." in the first message may be recognizing the identity note (shallow) or integrating it (deep). Test by asking an unexpected question that requires using identity knowledge in a novel context. "How would our collaboration handle a disagreement about methodology?" requires integrating identity with reasoning, not just reporting it.

---

## 8. What I Am NOT Seeing

**The biggest gap in my analysis:** I am drawing heavily on memory research and narrative identity theory, but the actual mechanism by which LLMs process in-context information is not well understood through cognitive science lenses. The transformer attention mechanism is not associative memory in the Hebbian sense. Positional encoding is not temporal context in Howard & Kahana's sense. The analogy is productive for design intuition but should not be mistaken for a mechanistic explanation.

**The measurement problem:** All my predictions about causal arcs, minimum identity structures, and schema competition are testable -- but testing them requires controlling for the model's inherent non-determinism. The same injection may produce partner-mode 60% of the time and tool-mode 40% of the time, and the variance may swamp the signal. I do not have a good solution for this beyond statistical power (many runs per condition).

**The philosophical gap:** I have sidestepped the question of whether AI persona reconstruction produces anything worth calling "identity" as opposed to "identity-consistent behavior." Cognitive science can measure behavior, not subjective experience. If the goal is functional identity continuity (the agent behaves as if it remembers), the tools I have described are sufficient. If the goal is something deeper, we are outside the domain of empirical science.

---

## References

- Anderson, J.R. (1983). *The Architecture of Cognition.* Harvard University Press.
- Anderson, J.R. & Schooler, L.J. (1991). Reflections of the Environment in Memory. *Psychological Science*, 2(6), 396-408.
- Baerger, D.R. & McAdams, D.P. (1999). Life story coherence and its relation to psychological well-being. *Narrative Inquiry*, 9(1), 69-96.
- Bartlett, F.C. (1932). *Remembering: A Study in Experimental and Social Psychology.* Cambridge University Press.
- Bjork, R.A. & Bjork, E.L. (1992). A new theory of disuse. In *From Learning Processes to Cognitive Processes*, Vol. 2, 35-67.
- Bjork, R.A. (1994). Memory and metamemory considerations in the training of human beings. In Metcalfe & Shimamura (Eds.), *Metacognition*, 185-205.
- Bruner, J.S. (1986). *Actual Minds, Possible Worlds.* Harvard University Press.
- Bruner, J.S. (1991). The narrative construction of reality. *Critical Inquiry*, 18(1), 1-21.
- Craik, F.I.M. & Lockhart, R.S. (1972). Levels of processing: A framework for memory research. *Journal of Verbal Learning and Verbal Behavior*, 11(6), 671-684.
- Honda, Y. et al. (2024). Human-Like Remembering and Forgetting in LLM Agents. *ACM CHI/HAI Conference*.
- Howard, M.W. & Kahana, M.J. (2002). A Distributed Representation of Temporal Context. *Journal of Mathematical Psychology*, 46(3), 269-299.
- Johnson, M.K., Hashtroudi, S., & Lindsay, D.S. (1993). Source monitoring. *Psychological Bulletin*, 114(1), 3-28.
- Mandler, G. (1980). Recognizing: The judgment of previous occurrence. *Psychological Review*, 87(3), 252-271.
- McAdams, D.P. (1985). *Power, Intimacy, and the Life Story.* Guilford Press.
- McAdams, D.P. (1993). *The Stories We Live By.* William Morrow.
- McAdams, D.P. (2001). The psychology of life stories. *Review of General Psychology*, 5(2), 100-122.
- McGaugh, J.L. (2000). Memory -- a century of consolidation. *Science*, 287(5451), 248-251.
- Packer, C. et al. (2023). MemGPT: Towards LLMs as Operating Systems. *arXiv:2310.08560*.
- Park, J.S. et al. (2023). Generative Agents: Interactive Simulacra of Human Behavior. *UIST 2023*.
- Pavlik, P.I. & Anderson, J.R. (2005). Practice and Forgetting Effects on Vocabulary Memory. *Cognitive Science*, 29(4), 559-586.
- Shinn, N. et al. (2023). Reflexion: Language Agents with Verbal Reinforcement Learning. *NeurIPS 2023*.
- Tulving, E. (1972). Episodic and semantic memory. In *Organization of Memory*, 381-403.
- Tulving, E. (2002). Episodic memory: From mind to brain. *Annual Review of Psychology*, 53, 1-25.
- Tulving, E. & Thomson, D.M. (1973). Encoding specificity and retrieval processes in episodic memory. *Psychological Review*, 80(5), 352-373.
- Yonelinas, A.P. (2002). The nature of recollection and familiarity. *Journal of Memory and Language*, 46(3), 441-517.

---

## Vault Notes Referenced

- `vaultmind-vault/concepts/episodic-memory.md` -- Tulving's episodic memory definition, autonoetic consciousness
- `vaultmind-vault/concepts/semantic-memory.md` -- Noetic consciousness, context-free knowledge, dissociation from episodic
- `vaultmind-vault/concepts/memory-consolidation.md` -- Synaptic and systems consolidation, sleep-dependent transfer
- `vaultmind-vault/concepts/schema-theory.md` -- Bartlett's reconstructive memory, schema-driven distortion
- `vaultmind-vault/concepts/base-level-activation.md` -- ACT-R activation equation, frequency and recency effects
- `vaultmind-vault/concepts/spreading-activation.md` -- Collins-Loftus model, network propagation
- `vaultmind-vault/concepts/temporal-activation-for-intermittent-systems.md` -- Compressed idle time, dual-strength model
- `vaultmind-vault/concepts/encoding-specificity.md` -- Tulving & Thomson, context-dependent retrieval
- `vaultmind-vault/concepts/levels-of-processing.md` -- Craik & Lockhart, depth of processing
- `vaultmind-vault/concepts/source-monitoring.md` -- Johnson et al., provenance and cryptomnesia
- `vaultmind-vault/concepts/interference-theory.md` -- Proactive/retroactive interference, fan effect
- `vaultmind-vault/concepts/working-memory.md` -- Context window as working memory analog
- `vaultmind-vault/concepts/multi-store-model.md` -- Atkinson-Shiffrin, three-store architecture
- `vaultmind-vault/concepts/generative-agents.md` -- Park et al., three-signal retrieval
- `vaultmind-vault/concepts/reflexion.md` -- Shinn et al., episodic self-reflection
- `vaultmind-vault/concepts/prospective-memory.md` -- Future intention memory
- `vaultmind-vault/concepts/cognitive-load-theory.md` -- Token budget as working memory constraint
- `vaultmind-vault/concepts/desirable-difficulties.md` -- Bjork's spaced retrieval benefits
- `vaultmind-vault/sources/source-bartlett-1932.md` -- War of the Ghosts, reconstructive memory
- `vaultmind-vault/sources/source-honda-2024.md` -- ACT-R for LLM agents, wall-clock decay limitation

---

**Note to parent agent:** I am a read-only Architect agent and cannot write files. The above content should be written to `/Users/peiman/dev/cli/vaultmind/docs/som/2026-04-11-persona-evaluation/round1/cognitive-scientist.md`. The directory exists and is empty.
