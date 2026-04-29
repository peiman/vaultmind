---
id: concept-membrane-potential
type: concept
title: Membrane Potential
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Resting potential
  - Transmembrane potential
  - Vm
tags:
  - neuroscience
  - electrophysiology
related_ids:
  - concept-neuron
  - concept-action-potential
  - concept-ion-channels
  - concept-synaptic-transmission
source_ids:
  - source-wikipedia-membrane-potential
  - source-wikipedia-resting-potential
  - source-hodgkin-huxley-1952
  - source-kandel-2012
---

## Overview

The membrane potential (Vm) is the voltage difference across a cell's plasma membrane, conventionally measured as inside relative to outside. It exists because [[ion-channels|Ion Channels]] in the membrane are selectively permeable to specific ions, and those ions sit at unequal concentrations on the two sides. A typical [[neuron|Neuron]] at rest sits near -70 mV (some sources cite -65 to -80 mV depending on cell type and species).

The resting potential is not a passive equilibrium of all ions — it is a steady state in which active pumping balances passive leak. Two layers of machinery maintain it: (1) ion-selective channels that let K+, Na+, Cl-, etc. flow toward their individual equilibrium potentials, and (2) the Na+/K+ ATPase, which actively extrudes 3 Na+ and imports 2 K+ per ATP, restoring the gradients that the leak currents would otherwise dissipate.

## Key Mechanisms

- **Equilibrium (Nernst) potential:** For a single permeant ion, the voltage at which net flow across the membrane is zero. Computed by the Nernst equation: `E_ion = (RT/zF) · ln([ion]_out / [ion]_in)`. At body temperature this gives roughly E_K ≈ -90 mV, E_Na ≈ +60 mV, E_Cl ≈ -70 mV, E_Ca ≈ +120 mV.
- **Goldman-Hodgkin-Katz (GHK) equation:** When several ions are simultaneously permeant, the resting Vm is a permeability-weighted blend of their equilibrium potentials. Because the resting membrane is most permeable to K+, the resting voltage sits closest to E_K — but Na+ leak pulls it slightly positive of pure E_K, giving the measured ~ -70 mV.
- **Sodium-potassium pump:** Maintains the [Na+] and [K+] gradients against leak. Slightly electrogenic (net 1 charge out per cycle), contributing a few mV of additional negative tilt.
- **Driving force:** For a given ion at voltage Vm, the driving force is `(Vm - E_ion)`. This determines the direction and magnitude of current when the corresponding channels open. For example, opening Na+ channels at rest produces strong inward Na+ current because (-70 - (+60)) = -130 mV of driving force into the cell.
- **Polarization vocabulary:** *Depolarization* = Vm becomes less negative (toward 0 or positive). *Repolarization* = return toward rest after a depolarization. *Hyperpolarization* = Vm becomes more negative than rest.

## Connections

The membrane potential is the resting state out of which every other electrical event in a neuron is generated. The [[action-potential|Action Potential]] is a transient, regenerative deviation of Vm produced when voltage-gated [[ion-channels|Ion Channels]] open in a coordinated sequence. [[synaptic-transmission|Synaptic Transmission]] modulates Vm in graded ways (EPSPs and IPSPs) that integrate at the soma. The Hodgkin-Huxley framework (1952) is the canonical link between the GHK/Nernst description of Vm and the dynamics of the spike.
