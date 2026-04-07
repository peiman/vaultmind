---
id: concept-chunking-strategies
type: concept
title: Chunking Strategies
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Document Chunking
  - Text Segmentation for Retrieval
tags:
  - retrieval
  - preprocessing
  - architecture
related_ids:
  - concept-retro
  - concept-rag
  - concept-embedding-based-retrieval
source_ids: []
---

## Overview

Chunking is the process of splitting source documents into segments before indexing them for retrieval. The choice of chunking strategy directly determines the granularity at which retrieval operates, and different granularities produce different precision-recall trade-offs. Too large a chunk and the retrieval system returns passages that are only partially relevant, polluting the context with off-topic content. Too small a chunk and retrieved segments lack the surrounding context needed to interpret them, and related information spread across adjacent chunks may be missed.

The retrieval literature has converged on several principal chunking strategies, ranging from simple mechanical splits to semantically aware segmentation:

**Fixed-size chunking** splits documents at a fixed token or character count, with optional overlap between consecutive chunks. This is simple and fast but ignores linguistic structure — chunks may split mid-sentence or mid-argument. The RETRO architecture (Borgeaud et al., 2022) uses 64-token chunks, a deliberately small granularity suited to a database of trillions of tokens where extremely fine-grained retrieval is beneficial.

**Sentence-level chunking** splits at sentence boundaries detected by a parser or heuristic. Each chunk is one or a small number of complete sentences. Retrieval at sentence level offers high precision but low context: the retrieved sentence may be meaningless without the surrounding paragraph.

**Paragraph-level chunking** treats each paragraph as a retrieval unit. This is a natural fit for many document types — paragraphs are typically coherent units of thought. Retrieval at paragraph level balances precision and context better than either fixed-size or sentence-level for typical prose.

**Semantic chunking** detects topic boundaries within a document using embedding similarity between consecutive segments: when the cosine similarity between adjacent segment embeddings drops below a threshold, a chunk boundary is inserted. This produces chunks that are internally coherent in topic, at the cost of higher preprocessing compute.

**Recursive or hierarchical chunking** applies chunking iteratively: start with large chunks, then recursively split any chunk that exceeds a coherence or size threshold until each chunk passes a coherence criterion. LlamaIndex popularized this as a practical default strategy for RAG preprocessing.

## Key Properties

- **RETRO's 64-token chunks:** The RETRO system uses 64-token fixed chunks over a 2-trillion token database. This small chunk size is deliberate — at retrieval database scale, small chunks enable more precise retrieval of specific facts. However, each chunk is retrieved alongside its neighbor (the two consecutive 64-token chunks form a "chunk neighbor" pair), partially restoring context.
- **Chunk overlap:** Fixed-size chunkers typically use 10–20% overlap between consecutive chunks to prevent relevant content from being split across two non-adjacent chunks with no overlap. This increases index size by 10–20% but substantially reduces boundary fragmentation artifacts.
- **Chunk size and embedding model:** Many embedding models are trained or fine-tuned on passages of a specific typical length. Using chunks much shorter or longer than the model's training distribution degrades embedding quality. Most commercial embedding APIs recommend 256–512 token chunks for optimal performance.
- **Parent-document retrieval:** A hybrid strategy where small chunks are used for retrieval (for precision) but the larger parent chunk or document is returned to the reader (for context). This decouples the retrieval granularity from the context granularity.

## Connections

Chunking interacts closely with [[embedding-based-retrieval|embedding-based retrieval]]: the embedding model encodes each chunk independently, and the chunk boundaries determine what semantic content is compressed into each vector. Semantic chunking attempts to align chunk boundaries with the embedding model's natural topic boundaries.

For [[retro|RETRO]], fixed-size 64-token chunking with neighbor retrieval is a core architectural choice, not just a preprocessing step — the retrieval is integrated into the transformer layers and designed around this specific granularity.

For [[rag|RAG]] systems, chunking is the first preprocessing decision and sets a ceiling on retrieval precision. A RAG system with well-chosen chunking and a strong retriever can outperform a system with a stronger retriever applied to poorly chunked documents.

For VaultMind, retrieval currently operates at note granularity — each Obsidian note is a single retrieval unit. This is appropriate for short notes, but for longer notes (project logs, research summaries, extended essays), note-level retrieval returns the entire document when only a paragraph is relevant. Sub-note chunking, splitting note bodies into paragraph-level segments for indexing, would improve precision for large notes. RETRO's 64-token chunks are too granular for personal vault notes; paragraph-level chunking is more appropriate given the typical structure and length of Obsidian notes. A parent-document strategy — retrieve at paragraph level, return the containing note section — would balance precision and context for [[context-pack|Context Pack]] assembly.
