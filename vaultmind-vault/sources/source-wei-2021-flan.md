---
id: source-wei-2021-flan
type: source
title: "Wei et al. Finetuned Language Models Are Zero-Shot Learners (FLAN, 2021)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2109.01652"
tags:
  - llms
  - deep-learning
  - instruction-tuning
related_ids:
  - concept-instruction-tuning
---

# Wei et al. — FLAN (2021)

Wei and colleagues at Google introduced instruction tuning: take a pre-trained LLM, then fine-tune it on a large mixture of NLP tasks each expressed as a natural-language instruction. The resulting FLAN-LaMDA-137B substantially improved zero-shot performance on unseen tasks, beating zero-shot GPT-3 on a majority of evaluation benchmarks.

The paper established that instruction-format multitask supervised fine-tuning is the bridge between raw pre-trained LMs and instruction-following assistants — the supervised step that precedes RLHF in the modern alignment stack. Together with T0 (Sanh et al. 2021) and Super-NaturalInstructions (Wang et al. 2022), FLAN is the root of the instruction-tuning literature.
