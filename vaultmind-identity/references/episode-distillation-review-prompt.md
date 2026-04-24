---
id: reference-episode-distillation-review-prompt
type: reference
title: "Episode Distillation Review — Prompt for a Future Session"
created: 2026-04-24
vm_updated: 2026-04-24
tags:
  - reference
  - prompt
  - plasticity
  - roadmap
related_ids:
  - reference-plasticity-priority-order
  - arc-plasticity-gap-from-inside
---

# Episode Distillation Review — Prompt for a Future Session

## When to use this

Paste the prompt below into a **new Claude Code session** after 2–3 working sessions have accumulated in `vaultmind-identity/episodes/`. The SessionEnd hook (shipped in PR #21) captures one episode per session automatically. The goal of the review is to inspect the real corpus and propose distillation shapes — what an arc-distillation layer should extract from episodes to produce arc drafts.

Per `reference-plasticity-priority-order`: episodic substrate ships first (done), then the distillation layer reads real episodes and proposes extraction rules. This prompt is step 2.

**Scheduled trigger**: 2026-04-26 (Sunday) — but only meaningful once there are ≥2 episodes in the corpus. If fewer, wait another session or two.

## The prompt (paste into a new session, verbatim)

---

You are reviewing the VaultMind episodic-memory corpus and proposing distillation rules. This is **analysis and proposal only** — do not implement, do not open PRs, do not modify files. Report only.

The episodes live at `vaultmind-identity/episodes/*.md` in this repo. Each file is a markdown capture of one Claude Code session between Peiman Khorramshahi (human, building VaultMind) and Claude (the AI mind VaultMind is a memory for). Schema: frontmatter with `session_id`, `started_at`, `ended_at`; sections for commits, PRs, files touched, user messages verbatim, assistant responses verbatim. These are the first episodes captured by the `vaultmind episode capture` feature shipped in PR #21 on 2026-04-24. The goal is an "arc-distillation" layer that reads episodes and proposes **arcs** — transformation records structured as trigger → push → deeper sight → principle. See `vaultmind-identity/principles/how-to-write-arcs.md` and existing arcs in `vaultmind-identity/arcs/*.md` for the arc format.

Your tasks:

1. **Corpus stats**: list all episodes, count them, report date range, total user messages, total commits, total PRs.
2. **Pattern mining**: grep across the episodes for recurring patterns. Specifically — (a) recurring user corrections (Peiman saying "no", "actually", "you should have", "stop", "don't"); (b) repeated assistant claims ("task check passes", "root cause is", "done"); (c) common surprise phrasings ("I didn't expect", "interesting", "unexpectedly"); (d) tool-use fingerprints of specific workflows (e.g. does "delegating to subagent → subagent blocked on permissions → main agent takes over" appear more than once?).
3. **Propose 3–5 distillation rules**: each rule names a pattern, explains what makes it arc-worthy (shift vs fact), and describes what fields the distillation would extract from the matching episode text.
4. **Sample arc draft**: pick one specific episode and write a draft arc that the rules above would have produced from it. Follow the arc shape from `principles/how-to-write-arcs.md`: trigger, push (with verbatim quote), deeper sight (first person), principle. Include the citation (transcript path + session id).

Report in a single response, under 1500 words. If the episodes directory has fewer than 2 episodes, say so and recommend rescheduling — the corpus is too thin for useful pattern mining.

---

## Context to include if the session feels thin

If the new session lacks context on why this review matters, point it at:

- `vaultmind-identity/arcs/plasticity-gap-from-inside.md` — the arc that motivated the whole episodic layer
- `vaultmind-identity/references/plasticity-priority-order.md` — the full roadmap; distillation is step 2
- `vaultmind-identity/principles/how-to-write-arcs.md` — the arc form the distillation aims to produce
- `internal/episode/episode.go` — what the parser actually extracts from a transcript

## Success criteria

The output is a good distillation-spec draft if:

- The proposed rules are specific enough to code against (regex, keyword sets, structural matches) — not vague heuristics.
- At least one rule names a pattern I'd have missed from reading arcs alone.
- The sample arc draft passes the `principles/how-to-write-arcs.md` verification loop (quoted push is verbatim, trigger/sight/principle are all present, cost-of-rule is visible).
- At least one proposed rule explicitly *rejects* a tempting-but-wrong pattern (e.g., "every commit gets an arc" — no, commits are facts).

If those four hold, the next ship is the distillation layer itself.
