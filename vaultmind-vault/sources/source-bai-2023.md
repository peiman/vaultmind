---
id: source-bai-2023
type: source
title: "Bai, Y., et al. (2023). LongBench: A Bilingual, Multitask Benchmark for Long Context Understanding. ACL 2024."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2308.14508"
aliases:
  - Bai 2023
  - LongBench paper
tags:
  - evaluation
  - benchmark
  - long-context
related_ids:
  - concept-longbench
  - concept-lost-in-the-middle
---

# Bai et al. — LongBench (2023/2024)

Bai et al. (THUDM group, Tsinghua University) introduced LongBench, the first bilingual multi-task benchmark for evaluating long-context understanding in large language models. The paper was posted to arXiv in August 2023 (arXiv:2308.14508) and published at ACL 2024. The benchmark covers 21 datasets spanning 6 task categories — single-document QA, multi-document QA, summarization, few-shot learning, synthetic tasks, and code — with 4,750 test instances. Average document length is 6,711 words (English) and 13,386 characters (Chinese).

## Key Findings

- **GPT-3.5-Turbo-16k leads open models but still degrades on length:** Even the strongest available model at the time showed performance decline as context length increased within the benchmark, confirming that context window size does not equal context utilization ability
- **Scaled position embedding and fine-tuning on longer sequences help:** Models specifically adapted for long contexts (via position scaling or training on long-document data) outperform base models on LongBench, suggesting these are productive directions for improvement
- **Context compression helps weak long-context models, not strong ones:** For models with limited long-context ability, retrieval-based compression (extracting relevant passages before prompting) substantially improves performance. For strong models (e.g., GPT-3.5-Turbo-16k), compression provided no benefit — the model could already locate relevant information itself
- **Open-source models lag significantly:** At the time of publication, open-source models with claimed long-context support fell well short of GPT-3.5 on most LongBench categories, particularly multi-document QA and summarization

## Relevance to VaultMind

The compression finding is the most directly actionable for VaultMind: the [[context-pack|Context Pack]] mechanism serves as context compression over the vault graph. For agents running smaller or open-source models, this compression step is architecturally necessary — not just a cost optimization. The benchmark also provides an external validation methodology: VaultMind's output quality could be measured by running LongBench-style multi-doc QA tasks over vault content and measuring whether context-pack retrieval improves performance relative to full-vault injection.

See [[longbench|LongBench]] for the full concept note.
