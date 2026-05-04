---
id: reference-citation-integrity-gate
type: reference
title: "Citation Integrity Gate — Verifier and Allowlist Design"
created: 2026-04-29
tags:
  - reference
  - research-vault
  - integrity
  - tooling
related_ids:
  - reference-plasticity-priority-order
  - reference-probe-before-commit
---

# Citation Integrity Gate

When the research vault grew from ~50 source notes to 155 in a single session via subagent waves, Peiman flagged a real risk: hallucinated citations and broken URLs. The fix is `scripts/verify_citations.py` plus `task check:citations`.

## What it checks

For every `vaultmind-vault/sources/*.md` frontmatter `url:`:

1. **arXiv URLs** — fetches the arxiv Atom API for the id, compares the paper's actual title to the source-note's `title:` field. Fails RED on title mismatch (the failure mode where the arxiv id resolves but points to a *different paper* than the citation claims).
2. **DOI URLs** — fetches CrossRef metadata API (free, no anti-bot wall — unlike doi.org redirects to publisher portals). Compares CrossRef's canonical title to the cited title. RED on mismatch, RED on 404, GREEN on match.
3. **Wikipedia URLs** — 200/404 only; the slug existing is enough.
4. **Other URLs** — 200/3xx GREEN, 404 RED. For known publisher portals that block automated checks (MIT Press, ACM, Harvard, PsycNet, Springer, ScienceDirect, OUP, Wiley, Nature, Science) the verifier retries with a browser User-Agent before giving up; if persistent, treats it as a known false-positive and passes.

## What "title match" means

`title_match(actual, cited)` returns true when the normalized `actual` is a substring of normalized `cited` (or vice versa), or when the two share at least 4 content words AND those words cover at least 50% of the shorter title. Strict enough to catch the Honda case (cited 2024 CHI denials paper but meant 2025 HAI ACT-R paper — zero meaningful word overlap), permissive enough to handle citation-style title fields that bundle author/year/journal into the title string.

## Bugs caught when it first ran on a 155-note vault

- `source-tulving-thomson-1973`: DOI was `10.1037/h0020356`. Real Encoding-Specificity paper is at `10.1037/h0020071`. Fixed.
- `source-honda-2024`: URL pointed to a 2024 CHI paper "AI language model can't" denials by Wester et al. The intended paper is Honda, Fujita, Zempo, Fukushima 2025 — *Human-Like Remembering and Forgetting in LLM Agents: An ACT-R-Inspired Memory Architecture* — at `10.1145/3765766.3765803`, HAI 2025. Renamed to `source-honda-2025` and updated two referencing concept notes.
- `source-ebbinghaus-1885`: DOI `10.5214/ans.0972-7531.1020103` does not exist in CrossRef. Replaced with the Internet Archive scan of the original 1913 English translation.
- `source-okeefe-dostrovsky-1971`: DOI was correct but the note's title field stripped the subtitle ("Preliminary evidence from unit activity in the freely-moving rat"), so the verifier flagged it as a near-mismatch. Restored full title.

## How to run it

```bash
task check:citations
```

The verifier is **not** in `task check` — it makes network calls (CrossRef + arxiv) and `task check` must stay offline. Run it explicitly when adding source notes, or wire into a pre-merge / CI step that has network access.

Exit code is nonzero if any RED is found, so CI can gate on it.

## Why this is "design over discipline" (manifesto principle 9)

Before this gate existed, citation integrity was honor-system: each subagent was *told* to use real DOIs. A subagent that hallucinated a DOI in good faith would slip through. The gate makes the rule mechanical — a subagent that hallucinates produces a RED, and the commit gets blocked. The discipline ("real citations only") is now a design property of the build pipeline, not a per-author commitment.

Same template as the other principle-9 fixes this session:
- `--pointers-only` (preload doesn't satisfy the dogfood rule by accident — body reads must be explicit)
- `RecordNoteAccess` wired to `Ask` (the access-tracking discipline now happens automatically)
- This: citation honesty enforced by network round-trip, not by trust.

## Allowlist evolution

`KNOWN_BOT_BLOCKED_HOSTS` in the verifier is a closed set today. New publisher portals that bot-block will surface as YELLOW or RED on first run; add them to the allowlist when verified-with-browser-UA returns 200. The allowlist is not a free pass — it only kicks in for hosts that already failed our normal UA, and only after we cross-validated that browser access works.
