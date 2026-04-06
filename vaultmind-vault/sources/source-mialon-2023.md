---
id: source-mialon-2023
type: source
title: "Mialon, G., et al. (2023). Augmented Language Models: a Survey. arXiv:2302.07842."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2302.07842"
aliases:
  - Mialon 2023
  - ALM Survey
tags:
  - llm-memory
  - survey
related_ids:
  - concept-augmented-language-models
  - concept-rag
  - concept-reflexion
---

# Mialon et al. — Augmented Language Models Survey (2023)

Mialon et al. (Meta AI Research) produced a comprehensive survey of language model augmentation, covering approximately 100 papers published through early 2023. The survey's primary contribution is a unified taxonomy that classifies augmentations into reasoning, tool use, and acting — previously fragmented across research communities.

The survey defines an Augmented Language Model as one that has been given capabilities beyond raw language modeling through at least one of: (1) reasoning strategies that decompose complex tasks, (2) access to external tools that can be called during generation, or (3) the ability to take actions that affect an environment. Memory systems (retrieval stores, knowledge bases, vector databases) are classified as tools under this taxonomy.

Key findings for the memory subfield: the survey documents that retrieval augmentation (RAG and related) consistently outperforms pure parametric models on knowledge-intensive tasks, with gains that scale with retrieval quality rather than model size alone. Tool use effectiveness depends critically on the quality of the tool's interface — poorly designed APIs degrade even capable models.

The survey explicitly covers: WebGPT, LaMDA, Toolformer, ReAct, SayCan, Inner Monologue, chain-of-thought, scratchpad, least-to-most prompting, and retrieval-augmented generation variants. It does not cover Voyager (published after submission) or MemGPT, which situates it as a pre-mid-2023 snapshot of a fast-moving field.

For VaultMind, this survey is the canonical reference for framing VaultMind as a memory tool in the ALM sense — an external system with a well-defined interface that a controller LLM invokes to retrieve structured knowledge. The interface design principles the survey documents directly inform VaultMind's CLI output format decisions.
