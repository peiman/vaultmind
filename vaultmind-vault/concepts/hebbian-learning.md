---
id: concept-hebbian-learning
type: concept
title: "Hebbian Learning"
status: active
created: 2026-04-09
tags:
  - neuroscience
  - plasticity
  - learning
related_ids:
  - concept-base-level-activation
  - concept-memory-consolidation
  - concept-spreading-activation
  - source-anderson-1983
---

## Overview

Hebbian learning is the principle that synaptic connections strengthen when pre- and post-synaptic neurons are co-activated — paraphrased as "neurons that fire together wire together." Donald Hebb proposed this in *The Organization of Behavior* (1949) as a theoretical mechanism for learning, decades before experimental confirmation.

The postulate: when neuron A repeatedly participates in firing neuron B, some growth process or metabolic change occurs such that A's efficiency in firing B is increased. This is a local, unsupervised learning rule — no external teacher is required. The synapse adapts based purely on the correlation between its input and output activity.

Long-term potentiation (LTP), discovered by Bliss and Lomo (1973) in rabbit hippocampus, provided the first direct evidence for Hebbian strengthening. LTP operates through NMDA receptor activation, calcium influx, and downstream cascades that increase synaptic efficacy. Its counterpart, long-term depression (LTD), weakens synapses that are not co-activated — implementing the "use it or lose it" principle.

## Key Properties

- Use-dependent strengthening: Repeated co-activation increases synaptic weight
- Locality: Each synapse adapts independently based on local activity
- Bidirectional: LTP strengthens used connections, LTD weakens unused ones
- Associativity: Weak inputs can potentiate if they coincide with strong inputs (basis for associative memory)
- The stability-plasticity dilemma: Too much plasticity erases old memories, too little prevents new learning

## Connections

Hebbian learning is the biological foundation for ACT-R's base-level activation equation: `Bi = ln(sum(tj^(-d)))`. Each retrieval of a memory chunk is analogous to synaptic activation — it strengthens the trace. Decay without use mirrors LTD. Anderson explicitly acknowledged this parallel, positioning ACT-R at Marr's computational level while Hebbian mechanisms operate at the implementation level.

For VaultMind, Hebbian learning motivates use-dependent note ranking: notes that are repeatedly retrieved should become more accessible (higher activation), while unused notes should fade. This is the theoretical basis for wiring the existing `access_count` and `last_accessed_at` schema fields into retrieval scoring.

The spreading activation retrieval mechanism (Collins & Loftus, 1975) is also grounded in Hebbian principles: activation spreads along associative links that were strengthened by co-occurrence during encoding.

## Sources

- Hebb, D.O. (1949). *The Organization of Behavior.* Wiley. DOI: 10.4324/9781410612403 (2002 reprint)
- Bliss, T.V.P. & Lomo, T. (1973). Long-lasting potentiation of synaptic transmission in the dentate area of the anaesthetized rabbit. *Journal of Physiology*, 232(2), 331-356. DOI: 10.1113/jphysiol.1973.sp010273
- Malenka, R.C. & Bear, M.F. (2004). LTP and LTD: An embarrassment of riches. *Neuron*, 44(1), 5-21. DOI: 10.1016/j.neuron.2004.09.012
