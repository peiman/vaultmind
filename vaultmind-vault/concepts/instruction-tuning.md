---
id: concept-instruction-tuning
type: concept
title: Instruction Tuning
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - SFT
  - Supervised Fine-Tuning
  - Instruction Following
  - FLAN
tags:
  - llms
  - deep-learning
  - alignment
  - instruction-tuning
related_ids:
  - concept-rlhf
  - concept-dpo
  - concept-gpt
  - concept-chain-of-thought
source_ids:
  - source-wei-2021-flan
---

## Overview

Instruction tuning is the supervised fine-tuning stage that turns a raw pretrained language model into something that follows natural-language instructions. The training data consists of (instruction, response) pairs covering a diverse mixture of tasks — summarization, translation, question answering, code, math, creative writing — with each task expressed in plain-language instruction form. The model is fine-tuned to imitate the responses given the instructions.

Introduced as a research program by [[source-wei-2021-flan|Wei et al. 2021]] (FLAN), with concurrent work T0 (Sanh et al. 2021) and Super-NaturalInstructions (Wang et al. 2022), instruction tuning is now a non-negotiable stage of every modern LLM pipeline. It is the SFT step that precedes [[concept-rlhf|RLHF]] / [[concept-dpo|DPO]] in the alignment stack.

## How It Works

**Training data.** A mixture of instruction-format datasets:
- **Crowd-sourced demonstrations** (OpenAI's InstructGPT data, Anthropic's HH-RLHF SFT split). Highest quality, smallest scale.
- **Reformatted academic NLP datasets** (FLAN, T0, Super-NaturalInstructions). Hundreds of tasks, each with multiple natural-language templates.
- **Self-generated** (Self-Instruct, Alpaca) — bootstrap a teacher model to generate instruction/response pairs, then filter.
- **Synthetic from a stronger model** (Orca, Phi, distilled-from-GPT-4 datasets). Dominant in 2023–25 open-weights training.
- **Conversational multi-turn** (ShareGPT-style logs, hand-curated dialogues). Necessary for chat behavior.

**Training objective.** Standard language modeling loss, but typically masked to apply only to the response tokens — the model is not trained to predict the instruction itself, only the answer given the instruction.

**Why it works.** Pretraining gives the model knowledge and language fluency; instruction tuning teaches it the instruction-response interaction pattern, calibrates its output format, and aligns its surface behavior with what users expect. The improvement on unseen tasks (zero-shot generalization) was the key surprising finding from FLAN — fine-tuning on a diverse instruction mixture transfers to held-out instruction types.

## Recent Developments

- **FLAN-T5 / FLAN-PaLM (2022)** — scaling instruction tuning across tasks, model sizes, and chain-of-thought data. Showed [[concept-chain-of-thought|CoT]] traces in instruction data substantially improve reasoning.
- **Self-Instruct (Wang et al. 2022) / Alpaca (Stanford 2023)** — bootstrap instruction data from a strong model, drastically lowering the cost of producing alignment datasets.
- **Open-source instruction datasets** — OpenHermes, UltraChat, WizardLM, Nectar, Tülu — community-curated mixtures became a competitive ecosystem.
- **Multi-turn instruction tuning** — explicit modeling of conversational state for chat assistants.
- **Tool-use SFT** — instruction data that includes function-call traces, training models for agentic use.
- **Reasoning-focused SFT** — long-CoT demonstration data became central to bootstrapping reasoning models before RL fine-tuning (DeepSeek-R1's "cold start" stage).
- **Quality-over-quantity (LIMA, 2023)** — 1000 carefully curated examples can rival huge instruction mixtures, suggesting instruction tuning surfaces capability rather than instilling it.

## Connections

Instruction tuning is the SFT step in the modern post-training stack: pretraining → instruction tuning → preference optimization ([[concept-rlhf|RLHF]] / [[concept-dpo|DPO]] / [[concept-constitutional-ai|RLAIF]]). It is the bridge between the raw [[concept-gpt|GPT-style]] pretrained completion engine and the chat assistants users interact with.

The LIMA finding — that 1000 examples can suffice — suggests instruction tuning is mostly about format and behavior elicitation rather than capability injection. The model already knows things from pretraining; instruction tuning teaches it which subset of that knowledge to surface and in what shape. This connects to the "alignment surfaces what's already there" framing that runs through alignment research.

For VaultMind, the analog is consistent prompt-formatting and few-shot patterning across the answer composition pipeline — the model produces better answers when its instruction context follows the same shape it was trained against.
