---
id: person-charles-packer
type: person
title: "Charles Packer"
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Packer
tags:
  - ai
  - llm-agents
  - memory-systems
related_ids:
  - concept-memgpt
  - source-packer-2023
url: "https://people.eecs.berkeley.edu/~cpacker/"
---

## About

Charles Packer is a PhD student at UC Berkeley and the lead author of MemGPT (2023), a system that gives LLMs the ability to manage their own memory hierarchy by paging information between a fixed context window and external storage — drawing an explicit analogy to operating system virtual memory management.

## Key Contributions

Packer's [[MemGPT]] architecture reframes the context window as a scarce resource to be managed rather than a fixed constraint to work around. The system's self-directed paging — where the LLM itself decides what to evict from context and what to retrieve from long-term storage — is a more autonomous version of what VaultMind's [[Context Pack]] command does on behalf of the user. His work highlights the importance of a well-defined read/write API for memory: VaultMind's note schema and `related_ids` graph are the structured substrate that would make an agent-managed memory loop tractable, since the LLM needs predictable handles to load and store information reliably.
