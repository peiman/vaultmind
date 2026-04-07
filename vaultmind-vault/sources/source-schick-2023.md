---
id: source-schick-2023
type: source
title: "Schick, T., et al. (2023). Toolformer: Language Models Can Teach Themselves to Use Tools. NeurIPS 2023."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2302.04761"
aliases:
  - Schick 2023
  - Toolformer paper
tags:
  - agent-architecture
  - tool-use
related_ids:
  - concept-toolformer
  - concept-react
  - concept-augmented-language-models
---

# Schick et al. — Toolformer (NeurIPS 2023)

Schick et al. (Meta AI) introduce Toolformer, a method for training language models to use external APIs in a self-supervised manner. The key contribution is that no human-labeled examples of tool use are required: the model bootstraps tool-use training data from its own predictions, filtered by whether tool calls improve the model's predictions of subsequent text.

The training procedure is a three-stage pipeline. First, a base LLM is prompted to annotate a text corpus with candidate API call positions and arguments. Second, all candidate API calls are executed and their results are retrieved. Third, the annotations are filtered: only calls where incorporating the API result reduces the perplexity of the subsequent tokens are kept. The model is then fine-tuned on the resulting dataset, learning to generate API calls inline as part of its text generation.

At inference time, the model generates text and inserts API calls (e.g., `[Calculator(14*17) → 238]`) at positions where it has learned they are useful. Results are retrieved at generation time and incorporated inline, allowing the model to perform arithmetic, retrieve facts, translate text, and look up calendar information as part of a single generation pass.

Experiments use a 6.7B GPT-J base model. Despite its relatively small size, Toolformer with tool access matches or exceeds GPT-3 (175B) and OPT (66B) on multiple zero-shot tasks including math QA, factual QA, multilingual tasks, and date/time tasks.

## Key Findings

- Self-supervised bootstrapping works: the perplexity-based filtering reliably identifies genuinely useful API calls, producing training data without human annotation
- Tool use scales well: a 6.7B model with tools outperforms 175B and 66B models without tools on zero-shot benchmarks, quantifying the practical value of tool access
- The model learns to use tools selectively — not every question triggers a tool call, only those where the tool would improve accuracy
- Calculator tool provides the largest absolute improvement on mathematical reasoning tasks, confirming that arithmetic is a consistent weakness of parametric LLMs
- Search tool provides the largest improvement on factual QA, demonstrating that knowledge retrieval is the most impactful external capability for open-domain question answering

## Relevance to VaultMind

Toolformer directly supports VaultMind's value proposition as an agent tool. The self-supervised training approach means that an agent could learn to use VaultMind's CLI without explicit human supervision — simply by observing which `vaultmind search` or `vaultmind recall` calls improved the quality of its downstream responses. The inline API call syntax Toolformer uses (`[API(args) → result]`) is also a natural fit for VaultMind's structured output format: retrieved notes could be incorporated inline as observations, exactly as Toolformer incorporates calculator or search results.
