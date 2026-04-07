---
id: concept-onnx-inference
type: concept
title: ONNX Inference
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - ONNX Runtime
  - ONNX in Go
tags:
  - architecture
  - inference
  - open-source
related_ids:
  - concept-open-source-embedding-models
  - concept-embedding-based-retrieval
source_ids: []
---

## Overview

ONNX (Open Neural Network Exchange) is an open format for representing machine learning models. Models trained in PyTorch, TensorFlow, or JAX can be exported to ONNX format and then executed by ONNX Runtime — a high-performance inference engine — without the original training framework installed.

ONNX Runtime (ORT) is the primary runtime for ONNX models. It is cross-platform, written in C++, and exposes bindings for Python, C#, Java, and Go. ORT supports CPU and GPU execution providers; for CPU inference, ORT is typically 2–5x faster than native PyTorch with no additional configuration.

**Go support via hugot:** The `knights-analytics/hugot` library provides native Go bindings for HuggingFace transformer pipelines using ONNX Runtime. It supports:
- Feature extraction (embedding) pipelines
- Text classification pipelines
- Token classification pipelines

The library loads ONNX-exported HuggingFace models and produces numerically identical predictions to the Python `transformers` inference path. `all-MiniLM-L6-v2` works out of the box with hugot via its pre-exported ONNX weights available on HuggingFace Hub.

## Key Properties

- **No Python dependency:** ONNX Runtime runs as a native C++ library. The hugot Go bindings link against ORT directly — no Python interpreter, no virtual environment, no pip.
- **Reproducible outputs:** ONNX export is deterministic. The Go hugot inference path produces bit-identical embeddings to the Python sentence-transformers path for the same model.
- **Build tag activation:** In hugot, the ONNX Runtime backend is activated via the `-tags ORT` build tag. Without this tag, the library compiles but falls back to a slower pure-Go implementation. Production builds should always include `-tags ORT`.
- **Model distribution:** ONNX model files for most HuggingFace transformer models are available as pre-exported `.onnx` files in model repositories (e.g., `model.onnx` in the `onnx/` subfolder). No Python export step is required.
- **Tokenizer parity:** hugot handles tokenization natively in Go using the HuggingFace tokenizer JSON file distributed with each model, ensuring the same tokenization as the Python reference implementation.
- **CPU performance:** all-MiniLM-L6-v2 via hugot+ORT embeds approximately 500–1000 sentences per second on a modern CPU core. A 123-note vault embeds in well under 2 seconds.
- **Model size:** The ONNX file for all-MiniLM-L6-v2 is approximately 22MB, making it practical to bundle with a CLI distribution or cache locally at first run.

## Connections

ONNX Inference is the enabling infrastructure for running [[open-source-embedding-models|Open-Source Embedding Models]] natively in Go without Python. This is what makes [[embedding-based-retrieval|Embedding-Based Retrieval]] practical in a Go CLI tool — the alternative (shelling out to a Python subprocess) introduces latency, dependency, and deployment complexity that are unacceptable in a CLI context. The hugot + ORT approach keeps VaultMind a single self-contained binary.

VaultMind v2: hugot + ONNX Runtime is the recommended path to local embeddings in Go. Build process: `go get github.com/knights-analytics/hugot`, add `//go:build ORT` build tags to embedding files, update `Taskfile.yml` to pass `-tags ORT` for builds involving the embedding pipeline. After Go upgrades, check that the hugot version is compatible with the ORT C++ library version. The ORT shared library (`libonnxruntime.so` / `libonnxruntime.dylib`) must be present at runtime — ship it alongside the binary or document the install step.
