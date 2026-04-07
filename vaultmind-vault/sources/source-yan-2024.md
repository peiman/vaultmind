---
id: source-yan-2024
type: source
title: "Yan, S.-Q., et al. (2024). Corrective Retrieval Augmented Generation. arXiv:2401.15884."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2401.15884"
aliases:
  - Yan 2024
  - CRAG paper
tags:
  - retrieval
  - self-correction
related_ids:
  - concept-corrective-rag
  - concept-rag
  - concept-self-rag
---

# Yan et al. — CRAG (2024)

Yan et al. introduce Corrective Retrieval Augmented Generation (CRAG), a method that adds a retrieval quality evaluator and adaptive corrective logic to standard RAG pipelines. The central observation is that RAG systems blindly trust their retriever: if retrieved documents are irrelevant or incorrect, the generator still receives and uses them, producing hallucinated or wrong outputs. CRAG addresses this by inserting a lightweight evaluator between retrieval and generation.

The evaluator classifies retrieved documents as correct (relevant), incorrect (irrelevant), or ambiguous (uncertain), and triggers different downstream behaviors for each case. Correct retrievals proceed to generation normally. Ambiguous retrievals trigger query refinement combined with web search augmentation. Incorrect retrievals cause full fallback to web search, discarding the original retrieved documents. In all cases, a decompose-then-recompose knowledge refinement step filters out irrelevant fine-grained knowledge strips before passing content to the generator.

Experiments span three open-domain QA tasks: PopQA, Bio (biography generation), and PubHealth (health misinformation detection). CRAG improves over standard RAG baselines across all tasks and works with both black-box LLMs (e.g., ChatGPT) and open-source fine-tuned models (e.g., LLaMA).

## Key Findings

- Retrieval evaluator alone provides consistent improvement: even without the web search fallback, filtering low-quality retrievals reduces generator hallucination
- Decompose-then-recompose refinement further improves results by removing irrelevant content that would otherwise dilute the generator's context
- Web search fallback for "incorrect" retrievals is the largest single contributor to improvement on queries where the local corpus is insufficient
- Plug-and-play: CRAG components can be added to any existing RAG pipeline without retraining the generator or retriever
- Performance gains are robust across model scales and architectures, indicating the technique is broadly applicable

## Relevance to VaultMind

CRAG motivates a retrieval quality gate for VaultMind. Rather than returning top-k results unconditionally, VaultMind's [[context-pack|context-pack]] and [[search|search]] commands could evaluate whether retrieved notes are genuinely relevant to the query before surfacing them. The decompose-then-recompose approach maps to note-level filtering: VaultMind could score individual note sections rather than whole notes, returning only the relevant portions. The three-branch logic (correct / ambiguous / incorrect) is a practical framework for VaultMind to expose retrieval confidence to agents, enabling them to decide whether to proceed with vault results or seek additional context.
