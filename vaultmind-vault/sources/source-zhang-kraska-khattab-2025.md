---
id: source-zhang-kraska-khattab-2025
type: source
title: "Zhang, Kraska, Khattab — Recursive Language Models (2025)"
created: 2026-04-09
vm_updated: 2026-04-09
url: "https://arxiv.org/abs/2512.24601"
aliases:
  - Zhang Kraska Khattab 2025
  - RLM paper
tags:
  - recursive-language-models
  - long-context
  - mcp
related_ids:
  - concept-a-rag
  - decision-vaultmind-mcp-server
---

# Zhang, Kraska, Khattab — Recursive Language Models (2025)

Zhang, Kraska, and Khattab (MIT) introduce Recursive Language Models (RLMs), a framework for handling prompts exceeding 10M tokens. Rather than ingesting the full context, an LLM treats the long prompt as an external environment and writes Python code to navigate it via a sandboxed REPL. A root LM orchestrates high-level planning while a recursive LM processes individual chunks, surfacing only what is needed for each reasoning step.

RLM-Qwen3-8B outperforms the base model by 28.3% on long-context benchmarks, demonstrating that structured navigation is more effective than brute-force context extension. The sandbox execution model means the LM can issue structured retrieval calls — keyword search, semantic lookup, range reads — against any tool that exposes a compatible interface.

VaultMind is a natural backend for RLM-style systems. The vault's structured note graph, activation-ranked context-pack, and MCP interface give a recursive LM precisely the tools it needs: fetch a note, traverse its graph neighbors, or assemble a context-pack from an activation query. See [[decision-vaultmind-mcp-server|MCP Server Decision]] for how VaultMind exposes these capabilities as MCP tools.

Citation: Zhang, M., Kraska, T., & Khattab, O. (2025). Recursive Language Models. arXiv:2512.24601. https://arxiv.org/abs/2512.24601
