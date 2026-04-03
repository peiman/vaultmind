---
id: source-collins-loftus-1975
type: source
title: "Collins & Loftus. A Spreading Activation Theory of Semantic Processing (1975)"
created: 2026-04-03
vm_updated: 2026-04-03
url: "https://doi.org/10.1037/0033-295X.82.6.407"
aliases:
  - Collins Loftus 1975
  - Spreading Activation paper
tags:
  - cognitive-science
  - retrieval
related_ids:
  - concept-spreading-activation
  - concept-semantic-networks
---

# Collins & Loftus — Spreading Activation (1975)

Collins and Loftus revised the earlier hierarchical semantic network model (Collins & Quillian 1969) to account for typicality and priming effects that a strict hierarchy could not explain. Their revised model represents concepts as nodes in a loosely organized network where link strength reflects semantic similarity and frequency of co-occurrence. When a concept is activated, energy spreads to connected nodes in proportion to link strength, priming them for faster retrieval—the process now called [[Spreading Activation]].

The model explains a wide range of empirical findings: why "robin" is verified as a bird faster than "penguin," why reading "doctor" speeds recognition of "nurse," and why indirect primes work if the associative path is short enough. The key theoretical move was abandoning strict hierarchical inheritance in favor of graded, weighted connections, which makes [[Semantic Networks]] far more expressive and psychologically realistic.

VaultMind implements a graph-based spreading activation pass over the vault's wikilink network. When a note is retrieved, activation spreads to its linked notes, boosting their retrieval scores for the remainder of the session. This means querying about one topic automatically surfaces related topics the user has previously connected, replicating the associative priming effects Collins and Loftus documented and giving VaultMind a form of contextual peripheral vision across the vault.
