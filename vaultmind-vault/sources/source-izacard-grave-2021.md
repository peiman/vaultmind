---
id: source-izacard-grave-2021
type: source
title: "Izacard, G., & Grave, E. (2021). Leveraging Passage Retrieval with Generative Models for Open Domain Question Answering. EACL 2021."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2007.01282"
aliases:
  - Izacard Grave 2021
  - FiD paper
tags:
  - retrieval
  - generative-qa
related_ids:
  - concept-fusion-in-decoder
  - concept-rag
  - concept-dense-passage-retrieval
---

# Izacard & Grave — Fusion-in-Decoder (EACL 2021)

Izacard and Grave addressed the question of how a generative model should aggregate information across many retrieved passages. Prior approaches either extracted a span from a single chosen passage (losing information from others) or concatenated all passages before encoding (hitting sequence length limits). FiD's solution is architecturally minimal: prepend each retrieved passage with the question, encode each (question, passage) pair independently with a T5 encoder, concatenate all resulting encoder hidden states, and pass them to the T5 decoder as a single extended cross-attention memory.

The independent encoding step is the key design choice. It avoids quadratic attention cost across all passages at encoding time while preserving each passage's full contextualization with the question. The decoder's cross-attention then performs the actual fusion, allowing the model to draw on any passage at any generation step. The paper showed that performance scales monotonically with the number of retrieved passages from 1 to 100, with the largest gains from 1→10 passages and continued improvement through 100.

At EACL 2021, FiD achieved state of the art on Natural Questions and TriviaQA open-domain QA benchmarks, outperforming contemporary extractive systems, RAG, and previous generative approaches. The paper used DPR as the retriever, making DPR+FiD the dominant baseline combination for open-domain QA research through 2022.

## Key Findings

- Performance improves monotonically with passage count (1 to 100 passages tested); no degradation from adding more passages
- Independent encoding is more effective than early fusion at scale: encoding all passages jointly hits T5's 512-token sequence limit, forcing truncation
- At 100 passages, FiD achieves 67.6% exact match on NaturalQuestions and 80.1% on TriviaQA — both state of the art at EACL 2021
- The architecture is drop-in compatible with any dense retriever; the paper tests with both DPR and BM25 retrievers
- Generative reading outperforms extractive reading when more passages are available, as the decoder can synthesize across passages rather than selecting one

## Relevance to VaultMind

FiD's independent-encode-then-fuse pattern is directly applicable to VaultMind's multi-note retrieval problem. When a query retrieves several related notes, VaultMind currently concatenates their content sequentially into the prompt, risking truncation for large note sets. FiD's architecture suggests encoding each note chunk with the query independently and letting the decoder synthesize across all of them — an approach that scales to large numbers of retrieved notes without hitting context limits. This is the recommended architecture for VaultMind's planned v2 embedding backend.
