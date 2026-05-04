---
id: concept-neural-manifolds
type: concept
title: Neural Manifolds
created: 2026-04-26
tags:
  - neuroscience
  - computational-neuroscience
  - population-coding
  - dimensionality-reduction
related_ids:
  - concept-grid-cells
  - concept-place-cells
  - concept-pyramidal-neurons
  - concept-predictive-coding
source_ids:
  - source-gallego-2017
---

## Overview

A **neural manifold** is a low-dimensional surface, embedded in the much higher-dimensional space of joint neural firing rates, on which the population activity of a brain area approximately lies during behaviour. Even when an experimenter records from hundreds or thousands of neurons, the joint activity does not fill the available dimensions — it occupies a small (typically 5-50 dimensional) curved subspace whose geometry reflects the structure of the task and of the underlying neural circuit.

The framework — pioneered in motor cortex by the Shenoy, Churchland, Cunningham, Sahani, Solla, and Miller labs — has reframed how neuroscientists think about coding. Instead of asking "what does each neuron represent?", it asks "what are the dominant population dynamics, and what trajectories do they trace through neural state space?". The canonical synthesis is [[source-gallego-2017|Gallego et al. (2017)]] in *Neuron*.

## How It Works

For a population of N neurons, each instantaneous pattern of firing rates is a point in N-dimensional space. Across time, the activity traces a trajectory through this space. Empirically, dimensionality-reduction methods (PCA, factor analysis, GPFA, demixed PCA, jPCA, dPCA, autoencoders) consistently find that >80-95% of the across-trial variance in motor, prefrontal, parietal, and sensory cortices is captured by a small number of dimensions — the *neural modes*. The set of points reachable by varying activations of these modes is the manifold.

Manifolds typically have a curved, often nonlinear structure. Trajectories on them correspond to the unfolding of behaviour: in motor cortex, an upcoming reach prepares the population by moving it to a specific point on the manifold ("preparatory state"), and the reach itself is the rotational dynamics that follow. In hippocampus, the manifold encodes position; in head-direction cells, it is literally a ring (Kim et al. 2017, *Nature*).

## Key Findings

- **Low-dimensional structure:** Population activity in motor and premotor cortex during reaching is well-captured by ~10-30 dimensions despite thousands of recorded neurons (Churchland et al. 2012; [[source-gallego-2017|Gallego et al. 2017]]).
- **Rotational dynamics:** Reaching produces rotational trajectories on the motor manifold (Churchland et al. 2012, *Nature*); the rotation, not the individual cell tuning, is the cleanest description of motor cortex output.
- **Stable across days and animals:** Gallego et al. (2018, 2020) showed that motor manifolds are stable across days in the same monkey and even across animals trained on the same task — neuron identities turn over, but the manifold geometry persists.
- **Topology mirrors task geometry:** The head-direction system in the *Drosophila* central complex (Kim et al. 2017; Seelig & Jayaraman 2015) and the rodent thalamus (Chaudhuri et al. 2019) form ring manifolds; the navigation system forms torus manifolds (Gardner et al. 2022, *Nature*) — population geometry directly mirrors the topology of the variable being represented.
- **Manifolds in deep networks:** ANNs trained to perform the same tasks develop low-dimensional internal manifolds with similar geometry (Sussillo & Barak; Mante et al. 2013), supporting the view that low-dimensional dynamics is a generic solution.

## Recent Developments

- **Cross-task manifold stability:** Safaie et al. (2023, *Nature*) showed that across different motor tasks, the underlying manifold remains shared and tasks add or rotate rather than rebuild it, suggesting the manifold is a long-lived computational substrate.
- **Manifold-aligned BCIs:** Brain-computer interface decoders that are aligned to the subject's motor manifold are dramatically more stable across days than decoders that ignore manifold structure (Degenhart et al. 2020).
- **Toroidal grid-cell manifold:** Gardner et al. (2022, *Nature*) used persistent homology on grid-cell ensemble activity to show that the activity lives on a torus — the cleanest empirical confirmation of theoretical attractor models for [[grid-cells|grid cells]].
- **A neural manifold view of the brain (Langdon et al. 2025, *Nat. Neurosci.*):** A recent perspective consolidating the manifold framework as a brain-wide organising principle, not just a motor-cortex curiosity.

## Connections

Neural manifolds are the population-level abstraction that includes [[place-cells|Place Cells]] (which span a position manifold), [[grid-cells|Grid Cells]] (which span the elegant toroidal manifold of Gardner et al. 2022), and [[time-cells|Time Cells]] (which span a temporal manifold). They sit naturally with [[predictive-coding|Predictive Coding]] / [[free-energy-principle|Free Energy Principle]] accounts, where the generative model lives on a low-dimensional latent manifold.

For VaultMind, the manifold idea is a reminder that high-dimensional knowledge stores often live on much lower-dimensional structure, and that the right query is one that aligns with the dominant modes — exactly what a well-trained embedding space tries to do. Dimensionality reduction over the vault (PCA / UMAP / diffusion maps over note embeddings) is the engineered counterpart of identifying the manifold a brain area lives on.
