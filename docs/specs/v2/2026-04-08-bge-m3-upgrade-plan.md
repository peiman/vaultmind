# BGE-M3 3-in-1 Upgrade — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all-MiniLM-L6-v2 with BGE-M3 for dense+sparse+ColBERT retrieval using pure Go backend, no CGO.

**Architecture:** BGEM3Embedder bypasses hugot's FeatureExtractionPipeline postprocessing to access raw per-token hidden states, then applies three heads (dense CLS pooling, sparse linear projection, ColBERT linear projection) as pure Go matrix ops. Two new BLOB columns for sparse and ColBERT storage. Two new retrievers (SparseRetriever, ColBERTRetriever) plug into the existing N-way RRF HybridRetriever for 4-way fusion.

**Tech Stack:** Go, hugot v0.7.0, nlpodyssey/gopickle, SQLite (modernc.org/sqlite), testify, cobra/viper

**Spec:** `docs/specs/2026-04-08-bge-m3-upgrade-design.md`

**Security note:** This plan loads PyTorch .pt weight files from BAAI's official HuggingFace repository using gopickle. These files contain only numeric tensor data (Linear layer weights/biases). The gopickle library parses tensor structures in pure Go. This approach was explicitly approved by the user during design (option A).

---

## 11 Tasks

1. Dense/Sparse/ColBERT head math (pure Go, no model needed)
2. Sparse and ColBERT encode/decode helpers
3. Schema migration + sparse/ColBERT DB storage
4. SparseRetriever
5. ColBERTRetriever
6. PyTorch weight loading via gopickle
7. Model download helper
8. BGEM3Embedder (wires everything together)
9. Extend EmbedNotes for 3-output embedders
10. Config + command wiring
11. Final integration verification

See the full task details in the design spec and code review conversation. Each task follows TDD with atomic commits.
