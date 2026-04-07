---
id: concept-prospective-memory
type: concept
title: Prospective Memory
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Future Intention Memory
  - Remembering to Remember
tags:
  - cognitive-science
  - memory-systems
related_ids:
  - concept-episodic-memory
  - concept-working-memory
  - concept-metamemory
source_ids: []
---

## Overview

Prospective memory is the cognitive capacity to remember to carry out an intended action at some point in the future. It is distinct from retrospective memory — remembering facts or past events — in that the target of recall is not something that happened but something that must still happen. Colloquially it is "remembering to remember."

The field distinguishes two core types based on what triggers the retrieval of the intention:

- **Event-based prospective memory:** Retrieval is cued by a specific external or internal event — "when I see the pharmacy, I must pick up the prescription." The intention remains dormant until the trigger stimulus is encountered.
- **Time-based prospective memory:** Retrieval is cued by reaching a target time — "at 3pm I must call the doctor." This requires ongoing self-monitoring of time, making it more cognitively demanding than event-based tasks because no external cue prompts retrieval.

## Key Properties

- **Encoding the intention:** The action and its trigger must be bound together and stored, typically drawing on [[episodic-memory|Episodic Memory]] for the "when/what" pairing
- **Retention interval:** The intention must survive across an ongoing delay period during which other cognitive work is performed
- **Trigger recognition:** When the cue condition is met, the dormant intention must be retrieved — this often involves [[metamemory|Metamemory]] monitoring
- **Execution:** The intended action must be initiated at the right moment; failures can occur at any stage (encoding, retention, recognition, or execution)
- **Attentional demands:** [[working-memory|Working Memory]] resources are involved, particularly for time-based tasks; divided attention reliably impairs prospective recall

## Connections

Prospective memory is an active area of research connecting cognitive science, neuropsychology (frontal lobe damage reliably impairs it), and applied settings such as medical compliance and aviation checklists. Einstein & McDaniel (1990) established the event-based/time-based taxonomy that remains the standard framework.

VaultMind agent task planning is an instance of prospective memory: the agent encodes an intention ("search for X on step 3"), retains it through intervening steps, and must execute it when conditions are met. A future VaultMind feature could implement a "reminder" note type — a note that surfaces automatically when a specified trigger condition is met (e.g., when a related note is accessed, a pending action note appears in the context pack). This would offload prospective memory demands from the agent onto the vault infrastructure.
