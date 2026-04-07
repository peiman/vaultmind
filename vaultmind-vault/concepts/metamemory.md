---
id: concept-metamemory
type: concept
title: Metamemory
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Memory About Memory
  - Metacognitive Memory
tags:
  - cognitive-science
  - metacognition
  - memory-systems
related_ids:
  - concept-desirable-difficulties
  - concept-source-monitoring
  - concept-working-memory
source_ids: []
---

## Overview

Metamemory refers to knowledge and monitoring of one's own memory system — its capacities, limitations, contents, and the strategies that improve its performance. The term was introduced by John Flavell (1971) in the context of children's understanding of memory development, but the cognitive science framework was systematized by Nelson and Narens (1990) who distinguished two levels: an object level (the memory system itself performing encoding, storage, and retrieval) and a meta level (processes that monitor and control the object level).

Metamemory encompasses a family of phenomena studied under different labels:

- **Feeling of Knowing (FOK):** A judgment that an unrecalled item could be recognized if presented — the sense that the answer is there even if it can't be retrieved right now. FOK predicts future recognition accuracy above chance.
- **Judgment of Learning (JOL):** A prediction made during or after study about how well an item will be remembered later. JOLs are often overconfident when made immediately after study (due to current fluency) but more accurate when delayed (because retrieval itself is a test).
- **Tip-of-the-Tongue (TOT) states:** Partial retrieval — knowing that one knows something and having access to peripheral features (first letter, number of syllables, related words) without the target. TOT states provide evidence that retrieval is a reconstructive search process, not all-or-nothing.
- **Monitoring accuracy:** The degree to which metamemory judgments (FOK, JOL) predict actual memory performance. A well-calibrated metamemory system allocates study time efficiently — more study for items flagged as weakly learned.

## Key Properties

- **Nelson-Narens framework:** Monitoring (meta → object: sampling the state of memory) and control (meta → object: regulating encoding/retrieval strategies based on monitoring output) are the two core metamemory functions
- **Study time allocation:** The primary adaptive function of metamemory — directing more cognitive effort toward poorly encoded or weakly retrieved items
- **Calibration vs. resolution:** Two distinct dimensions of metamemory accuracy; calibration measures whether confidence matches accuracy overall, resolution measures whether high-confidence items are more accurate than low-confidence ones
- **Illusions of knowing:** Familiarity with material (fluency) can be mistaken for actual knowing — a major source of metamemory miscalibration that [[desirable-difficulties|Desirable Difficulties]] are partly designed to counteract

## Connections

VaultMind's `doctor` command is the system's metamemory subsystem — it monitors the health of the memory system itself rather than any particular memory's content. It reports orphaned nodes, missing `source_ids`, broken wikilinks, and coverage gaps: the systemic analog of monitoring accuracy and study-time allocation. Access tracking (planned for v2) will add per-note "feeling of knowing" metrics: notes frequently retrieved by the agent are operationally "well-known," while rarely accessed notes with high connectivity are flagged as candidates for explicit review — exactly the study-time allocation function metamemory serves in human learners. [[source-monitoring|Source Monitoring]] theory overlaps here: both metamemory and source monitoring are metacognitive processes that evaluate the quality and origin of memory records rather than their content.
