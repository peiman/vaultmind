---
id: concept-ion-channels
type: concept
title: Ion Channels
created: 2026-04-29
aliases:
  - Ion channel
  - Membrane channels
tags:
  - neuroscience
  - electrophysiology
  - ion-channels
related_ids:
  - concept-action-potential
  - concept-membrane-potential
  - concept-synaptic-transmission
  - concept-neurotransmitters
  - concept-neuron
source_ids:
  - source-wikipedia-ion-channel
  - source-hodgkin-huxley-1952
  - source-kandel-2012
---

## Overview

Ion channels are pore-forming integral membrane proteins that allow specific ions to cross the lipid bilayer down their electrochemical gradients. They are the elementary devices that make a [[neuron|Neuron]] electrically excitable: by opening and closing in response to controlled stimuli, they convert biological events (voltage changes, ligand binding, mechanical force) into transient ionic currents that change the [[membrane-potential|Membrane Potential]].

Two attributes characterize any channel: its **selectivity** (which ion it lets through — Na+, K+, Ca2+, Cl-, or non-selective cation) and its **gating** (what opens and closes it).

## Key Mechanisms

- **Voltage-gated channels** open in response to changes in membrane voltage. Voltage-gated Na+ channels (Nav) drive the rising phase of the [[action-potential|Action Potential]] with their fast activation and inactivation; voltage-gated K+ channels (Kv) drive repolarization; voltage-gated Ca2+ channels (Cav) couple presynaptic spike arrival to neurotransmitter release in [[synaptic-transmission|Synaptic Transmission]]. The Hodgkin-Huxley (1952) model is the canonical mathematical description of voltage-gated Na+ and K+ kinetics.
- **Ligand-gated channels** (ionotropic receptors) open when a [[neurotransmitters|Neurotransmitter]] or other small molecule binds. Examples include nicotinic acetylcholine receptors, ionotropic glutamate receptors (AMPA, NMDA, kainate), GABA-A receptors, and glycine receptors. They produce the fast EPSPs and IPSPs of postsynaptic responses.
- **Other gating modalities:** mechanosensitive channels (Piezo1/2 — touch, hearing), temperature-sensitive channels (TRP family), lipid-gated and second-messenger-gated channels (cyclic-nucleotide-gated channels in vision and olfaction).
- **Selectivity filter:** A short stretch of amino acids in the pore (e.g., the K+ channel's signature TVGYG) coordinates the dehydrated ion's charge and radius, allowing K+ to pass at near-diffusion-limited rates while excluding the smaller Na+. MacKinnon's structural work on K+ channels (Nobel 2003) clarified this geometry.
- **Sodium-potassium pump (Na+/K+ ATPase):** Not a channel — an active transporter — but its 3 Na+ out / 2 K+ in stoichiometry sets up the gradients that channels then exploit.

## Connections

Ion channels are the implementation layer beneath every other concept in this cluster. The resting [[membrane-potential|Membrane Potential]] is set by which channels are open at rest (mostly K+ leak channels). The [[action-potential|Action Potential]] is voltage-gated Na+ and K+ channels in concert. [[synaptic-transmission|Synaptic Transmission]] is voltage-gated Ca2+ channels in the presynaptic terminal plus ligand-gated channels on the postsynaptic side. [[neurotransmitters|Neurotransmitters]] are, from the channel's point of view, just the ligands that gate the ionotropic receptors.
