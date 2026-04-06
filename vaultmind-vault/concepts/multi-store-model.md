---
id: concept-multi-store-model
type: concept
title: Multi-Store Model
created: 2026-04-07
vm_updated: 2026-04-07
aliases:
  - Atkinson-Shiffrin Model
  - Modal Model of Memory
tags:
  - cognitive-science
  - memory-systems
  - memory-architecture
related_ids:
  - concept-working-memory
  - concept-episodic-memory
  - concept-semantic-memory
source_ids:
  - source-atkinson-shiffrin-1968
---

## Overview

The multi-store model (also called the modal model), proposed by Richard Atkinson and Richard Shiffrin in 1968, organized human memory into three structurally distinct stores connected by transfer processes. It became the dominant framework in memory research through the 1970s and remains the most widely cited structural model of memory architecture.

The three stores are: (1) the **sensory register**, which holds a brief, high-fidelity copy of perceptual input for roughly 0.5–2 seconds — iconic memory for vision (Sperling, 1960) and echoic memory for audition; (2) the **short-term store (STS)**, with a limited capacity of approximately 7 ± 2 items (Miller, 1956), retention of roughly 15–30 seconds without rehearsal, and susceptibility to displacement and interference; and (3) the **long-term store (LTS)**, characterized by effectively unlimited capacity and potentially permanent retention, though vulnerable to retrieval failure.

Transfer between stores is governed by attention (sensory → STS) and rehearsal (STS → LTS). The model introduced the key distinction between structural stores (fixed hardware) and control processes (flexible strategies like rehearsal, coding, and retrieval strategies). This distinction was theoretically important: it explained why some patients with brain damage could have intact LTS but impaired STS, and vice versa.

The model was influential but subsequently challenged on several fronts. The [[levels-of-processing|Levels of Processing]] framework (Craik & Lockhart, 1972) argued that memory durability depends on depth of processing, not on which store information passes through. Baddeley and Hitch (1974) showed that STS is not a unitary store but a multi-component system — replacing it with the [[working-memory|Working Memory]] model. Evidence also accumulated that LTS transfer does not require conscious rehearsal.

## Key Properties

- **Three discrete stores:** Sensory register → short-term store → long-term store, each with distinct capacity, duration, and encoding characteristics
- **Attention as gating mechanism:** Attended information passes from sensory register to STS; unattended information decays within the sensory register
- **Rehearsal as transfer mechanism:** Maintenance rehearsal keeps items active in STS; sufficient rehearsal transfers to LTS
- **Limited STS capacity:** ~7 ± 2 chunks (Miller, 1956), with displacement when capacity is exceeded
- **Serial flow model:** Information must pass through STS to reach LTS — though subsequent research has shown exceptions (implicit memory, semantic priming)

## Connections

The multi-store model maps directly onto the VaultMind agent architecture. The agent's context window corresponds to STS — limited in capacity, volatile, and requiring active maintenance. VaultMind itself is the LTS — persistent, high-capacity, and structurally organized. The [[context-pack|Context Pack]] mechanism is the transfer process analogous to rehearsal: it moves information from LTS into the agent's context window (STS) for active reasoning.

This analogy also highlights VaultMind's current gap: the Atkinson-Shiffrin model includes a feedback loop where active processing in STS can strengthen LTS representations. VaultMind has no equivalent — the agent cannot yet write back to the vault mid-session to consolidate new understanding into LTS. The `note create` command begins to address this, but a full rehearsal-to-consolidation loop remains a v2 concern.
