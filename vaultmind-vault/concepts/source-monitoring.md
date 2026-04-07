---
id: concept-source-monitoring
type: concept
title: Source Monitoring
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Source Monitoring Framework
  - SMF
tags:
  - cognitive-science
  - memory-systems
  - provenance
related_ids:
  - concept-episodic-memory
  - concept-schema-theory
  - concept-encoding-specificity
source_ids:
  - source-johnson-1993
---

## Overview

Source monitoring, formalized by Marcia Johnson, Shahin Hashtroudi, and D. Stephen Lindsay (1993), describes the cognitive processes by which people identify the origins of their memories, knowledge, and beliefs. The central observation is that memories do not carry explicit source tags — the brain does not record "this came from source X" as a separate field. Instead, source is inferred at retrieval from the characteristic features of the memory trace: perceptual detail (vivid sensory information suggests external origin), semantic detail, spatial and temporal context, cognitive operations (reasoning steps suggest internally generated content), and affective reactions.

Because source is inferred rather than stored, errors are systematic and predictable. Failures in source monitoring explain a wide range of memory distortions.

## Key Properties

- **External vs. internal source monitoring:** Distinguishing between memories of perceived events (external) and memories of imagined, thought, or inferred events (internal)
- **Between-external source monitoring:** Distinguishing which of two external sources a memory came from — which person said something, which text contained a fact
- **Reality monitoring:** A specific case of external/internal distinction — did this happen, or did I imagine it?
- **Cryptomnesia:** A source monitoring failure in which a previously encountered idea is re-generated as if novel, because the source memory has been lost but the content persists
- **Fiction-to-fact incorporation:** Source monitoring errors that cause fictional or hypothetical content to be mistakenly attributed to reality — dangerous in AI systems that generate plausible-sounding content
- **Characteristics that support accurate source attribution:** Distinctiveness of the encoding context, source salience at encoding, low similarity between sources, conscious attention to source at encoding

## Connections

Source monitoring maps directly to VaultMind's provenance tracking architecture. Biological memory lacks explicit source tags — VaultMind deliberately supplies them. The `source_ids` field on every note is a machine-readable source attribution. Edge `origin` fields and `confidence` levels provide fine-grained provenance for relationships, not just facts. This design directly addresses the failure mode that source monitoring theory identifies: without explicit tags, systems (biological or artificial) will infer source from content features — a process that generates systematic errors. VaultMind's [[episodic-memory|episodic memory]] analog stores not just what was recorded but when, from what context, and with what confidence — the information that source monitoring theory says is critical for accurate attribution. Cryptomnesia in LLM agents (presenting training knowledge as vault-derived knowledge) is a source monitoring failure that explicit `source_ids` help prevent.
