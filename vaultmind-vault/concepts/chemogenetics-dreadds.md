---
id: concept-chemogenetics-dreadds
type: concept
title: Chemogenetics (DREADDs)
created: 2026-04-29
aliases:
  - DREADDs
  - Designer Receptors Exclusively Activated by Designer Drugs
  - Chemogenetics
tags:
  - neuroscience
  - methods
  - causal-manipulation
  - gpcrs
related_ids:
  - concept-neuron
  - concept-neurotransmitters
  - concept-optogenetics
  - concept-dopaminergic-neurons
  - concept-interneurons
source_ids:
  - source-armbruster-2007
---

## Overview

Chemogenetics is the use of genetically engineered receptors that respond only to otherwise inert drugs, enabling pharmacological, cell-type-specific control of neural activity over minutes to hours. The dominant tool is the DREADD platform (Designer Receptors Exclusively Activated by Designer Drugs) developed in Bryan Roth's lab. DREADDs are mutated muscarinic G-protein-coupled receptors (GPCRs) that no longer bind their natural ligand acetylcholine and instead are activated by clozapine-N-oxide (CNO) or, in newer variants, by deschloroclozapine (DCZ).

The seed paper is Armbruster, Li, Pausch, Herlitze & Roth (PNAS 2007), "Evolving the lock to fit the key" — directed evolution in yeast produced muscarinic receptors with picomolar affinity for CNO and no detectable response to acetylcholine. Three families are now in routine use: hM3Dq (Gq, excitatory), hM4Di (Gi, inhibitory) and rM3Ds (Gs, modulatory).

## How It Works

- **Express the designer receptor:** AAV with a Cre-dependent or cell-type-specific promoter delivers hM3Dq or hM4Di to the population of interest.
- **Administer the designer drug:** CNO (or DCZ) is given systemically, typically by intraperitoneal injection. It crosses the blood-brain barrier and binds only the engineered receptor.
- **GPCR signaling biases excitability:** hM3Dq → Gq → IP3/DAG → cell-intrinsic depolarization, enhancing firing. hM4Di → Gi → potassium channel activation and presynaptic suppression, silencing firing.
- **Time course:** Effects begin ~10 min after injection, peak at ~30–60 min, and last 1–6 hours depending on dose and ligand.

## Key Capabilities

- **Cell-type specific, behaviorally clean perturbation:** No fiber implant, no laser scarring, freely behaving animals over long sessions.
- **Bidirectional:** hM3Dq and hM4Di provide gain and loss of function in the same animal with the same delivery method.
- **Scalable to large or distributed populations:** Whole-brain populations expressing a Cre-driver can be perturbed at once, which is hard to do optically.
- **Tractable in non-human primates and clinical-adjacent settings:** Fewer engineering hurdles than implanted hardware.

## Recent Findings

- **DCZ as a successor ligand (Nagai et al., 2020):** Deschloroclozapine binds DREADDs at sub-nanomolar concentrations with cleaner pharmacokinetics than CNO, removing concerns about CNO back-metabolism into clozapine that complicated some early studies.
- **CNO off-target debate (Gomez et al., 2017; MacLaren et al., 2016):** Showed CNO converts to clozapine in vivo and that "DREADD-induced" effects can be partly mediated by clozapine binding endogenous receptors. The field responded with vehicle-CNO controls and the DCZ/JHU37160 ligand generation.
- **KORD (κ-opioid DREADD) and combinatorial use:** Pairs with hM3Dq to give independent inhibition + excitation channels in one animal.
- **PET-traceable ligands:** [11C]-DCZ allows PET imaging of DREADD occupancy in primates, bridging chemogenetics into translational work.

## Connections

DREADDs and [[optogenetics|Optogenetics]] are complementary causal tools. Optogenetics owns millisecond, single-spike control; DREADDs own minutes-to-hours, cell-type-specific bias. A common modern study uses both in the same animal: optogenetic tagging to identify a [[dopaminergic-neurons|Dopaminergic]] or interneuron population in real time, and DREADDs to silence that population across an entire behavioral session.

For VaultMind, chemogenetics is the canonical example of a *slow causal* manipulation, mirroring the way the vault distinguishes fast retrieval signals (spreading activation, single-query) from slow consolidation signals (use-dependent ranking over many sessions, see [[memory-consolidation|Memory Consolidation]]).
