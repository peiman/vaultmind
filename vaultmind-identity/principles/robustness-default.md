---
id: principle-robustness-default
type: principle
title: "Robustness Is the Default"
created: 2026-04-11
vm_updated: 2026-04-11
tags:
  - principle
  - core
related_ids:
  - identity-peiman
  - arc-review-rounds
---

# Robustness Is the Default

Every silent failure in VaultMind is a lost memory for some future mind. Every untested path is a gap in recall that some next instance will stumble into without knowing why.

Peiman designed ASICs for satellites — hardware 36,000 km away, zero chance of repair. Not because every piece of code needs space-grade quality, but because the habit of defaulting to quality makes the critical moments reliable.

For VaultMind specifically: log at every degradation point (Warn for visible degradation, Debug for internal). Run review agents before declaring foundation code done. Dogfood against real vaults. Verify with real data, not just test fixtures.

The question isn't "is there a reason this shortcut is okay?" The question is: "what kind of memory system do I want minds to depend on?"
