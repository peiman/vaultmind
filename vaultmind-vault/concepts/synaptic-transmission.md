---
id: concept-synaptic-transmission
type: concept
title: Synaptic Transmission
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Synaptic signaling
  - Neurotransmission
tags:
  - neuroscience
  - synaptic-transmission
related_ids:
  - concept-neuron
  - concept-action-potential
  - concept-neurotransmitters
  - concept-ion-channels
  - concept-hebbian-learning
source_ids:
  - source-wikipedia-chemical-synapse
  - source-kandel-2012
  - bliss-lomo-1973
---

## Overview

Synaptic transmission is the process by which a signal passes from one [[neuron|Neuron]] to the next at a synapse. The dominant form in the vertebrate brain is the chemical synapse: an [[action-potential|Action Potential]] arriving at the axon terminal opens voltage-gated calcium channels, the resulting Ca2+ influx triggers fusion of synaptic vesicles with the presynaptic membrane, and [[neurotransmitters|Neurotransmitters]] diffuse across the ~20 nm synaptic cleft to bind receptors on the postsynaptic membrane. A minority of synapses are electrical (gap junctions), where ions flow directly between coupled cells.

Chemical synapses are slower than electrical ones (millisecond synaptic delay vs. effectively instantaneous), but they are unidirectional, sign-flexible (a single transmitter can be excitatory or inhibitory depending on the receptor), and modifiable — which is what makes them the substrate for learning.

## Key Mechanisms

- **Presynaptic release:** Spike arrival → voltage-gated Ca2+ channels open → Ca2+ binds synaptotagmin on docked vesicles → SNARE-mediated exocytosis → quanta of transmitter released into the cleft.
- **Postsynaptic response:** Transmitter binds ionotropic and/or metabotropic receptors. Ionotropic receptors are themselves [[ion-channels|Ion Channels]] (ligand-gated) and produce fast postsynaptic potentials.
- **EPSP (excitatory postsynaptic potential):** Net inward current (e.g., Na+/Ca2+ through AMPA or NMDA receptors driven by glutamate) depolarizes the postsynaptic membrane toward threshold.
- **IPSP (inhibitory postsynaptic potential):** Net outward / shunting current (e.g., Cl- through GABA-A receptors) hyperpolarizes or stabilizes the postsynaptic membrane against firing.
- **Summation:** The postsynaptic neuron integrates many EPSPs and IPSPs across space (different synapses) and time (overlapping decays). Only when the soma/axon-hillock voltage crosses threshold does a postsynaptic spike fire.
- **Termination:** Transmitter action is ended by reuptake transporters, enzymatic degradation (e.g., acetylcholinesterase), or diffusion.
- **Plasticity:** The strength of a chemical synapse is not fixed. Long-term potentiation (LTP), discovered by Bliss and Lømo (1973) in rabbit hippocampus, was the first direct demonstration of activity-dependent synaptic strengthening.

## Connections

Synaptic transmission is the cellular implementation of communication between neurons in any computation the brain performs. It is downstream of the [[action-potential|Action Potential]] (which triggers it) and upstream of the next neuron's integration (which sums EPSPs and IPSPs against threshold). It is also the locus of [[hebbian-learning|Hebbian Learning]]: the use-dependent strengthening of co-active connections, captured experimentally as LTP and LTD, happens at the synapse — most famously through NMDA-receptor-dependent calcium signaling that modifies AMPA-receptor trafficking.

For VaultMind, the synapse is the biological analogue of a wikilink between notes: a directional connection whose effective strength can be modulated by use. Spreading activation across the vault graph is, in this sense, an information-retrieval analogue of postsynaptic summation.
