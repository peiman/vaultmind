---
id: concept-toolformer
type: concept
title: Toolformer
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Tool-Using LLM
  - Self-Supervised Tool Learning
tags:
  - agent-architecture
  - tool-use
  - llm-memory
related_ids:
  - concept-react
  - concept-augmented-language-models
  - concept-voyager
source_ids:
  - source-schick-2023
---

## Overview

Toolformer (Schick et al., Meta, NeurIPS 2023) demonstrates that a language model can learn, in a self-supervised manner, to use external APIs — deciding which API to call, when to call it, what arguments to provide, and how to incorporate the returned result into its generation. Unlike ReAct, which requires an explicit prompting loop and a human-designed action schema, Toolformer bootstraps tool use by having the model annotate its own training data with API calls and then filtering those annotations by whether they improve prediction quality.

The pipeline: (1) prompt a base LLM to suggest where API calls could be inserted into existing text, along with what arguments to use; (2) execute those API calls and retrieve results; (3) filter to keep only calls where the result actually reduces the perplexity of the subsequent text; (4) fine-tune on the filtered, annotated dataset. The result is a model that has internalized when and how to use tools as part of its normal generation process.

Tools used in the paper: a calculator, a QA system, two search engines (Wikipedia and BM25), a machine translation system, and a calendar. Despite being trained with 6.7B parameters (GPT-J scale), Toolformer matches or exceeds much larger models on zero-shot tasks by offloading computation to appropriate tools. arXiv:2302.04761.

## Key Properties

- **Self-supervised training:** No human-labeled examples of tool use required. The model bootstraps annotations from its own generations and filters by perplexity improvement.
- **Decides when to call:** Unlike systems that always call a tool, Toolformer learns when a tool call would actually help — calling tools less frequently when its parametric knowledge suffices.
- **Five tool types:** Calculator (arithmetic), QA system, search (Wikipedia, BM25), translator, calendar — demonstrating generality across retrieval, computation, and world-state tools.
- **Zero-shot competitive with larger models:** At 6.7B parameters, Toolformer outperforms GPT-3 (175B) on multiple zero-shot benchmarks, showing tool use compensates for scale differences.
- **Structured call syntax:** API calls are represented inline as special tokens in the generated text: `[API(args) → result]`, which the model learns to produce and consume naturally.
- **Perplexity filter:** Only API calls that reduce perplexity of surrounding text are retained for training, ensuring the model learns genuinely useful tool calls rather than random API invocations.

## Connections

Toolformer is closely related to [[react|ReAct]] but differs in its training approach: ReAct uses prompting and requires no training, while Toolformer fine-tunes the model to produce tool calls intrinsically. Both treat external tools as part of the model's action space.

[[augmented-language-models|Augmented Language Models]] provides the broader context in which Toolformer sits: one of several methods for equipping LLMs with external capabilities beyond their parametric knowledge.

[[voyager|Voyager]] extends the idea of learned tool use to open-ended skill acquisition, where the "tools" are programs the agent itself has written.

For VaultMind, Toolformer demonstrates that LLMs can learn to call external APIs as part of their generation process. VaultMind's CLI is exactly such an API — an agent trained in a Toolformer-style setup could learn when to call `vaultmind search`, `vaultmind recall`, or `vaultmind context-pack`, with what arguments, and how to incorporate the returned note content into its response. The self-supervised training approach is particularly appealing because it could be applied to a user's own conversation history with VaultMind to learn personalized retrieval patterns.
