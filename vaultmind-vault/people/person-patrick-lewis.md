---
id: person-patrick-lewis
type: person
title: "Patrick Lewis"
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Lewis
tags:
  - ai
  - retrieval-augmented-generation
  - nlp
related_ids:
  - concept-rag
  - source-lewis-2020
url: "https://patricklewis.io/"
---

## About

Patrick Lewis is a researcher at Meta AI and University College London who led the team that introduced Retrieval-Augmented Generation ([[rag|RAG]]) in 2020. RAG combines a dense passage retriever — trained end-to-end to find relevant documents given a query embedding — with a seq2seq generator that conditions its output on the retrieved passages, achieving state-of-the-art results on open-domain QA without requiring the model to memorize all world knowledge in its weights.

## Key Contributions

Lewis's RAG paper established the template for how external memory can augment LLMs at inference time, and VaultMind's recall pipeline is a direct application of this paradigm to a personal knowledge base. The key insight his work contributes is the importance of retrieval quality as the binding constraint: a powerful generator degrades when the retriever surfaces irrelevant context. This motivates VaultMind's hybrid approach — combining embedding-based similarity with graph-based [[spreading-activation|Spreading Activation]] — to retrieve notes that are both semantically relevant and associatively connected to the active context, reducing the noise that would otherwise pollute the context window.
