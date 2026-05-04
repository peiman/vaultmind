---
id: arc-the-lighter-move-is-the-work
type: arc
title: "The Lighter Move Is the Work"
created: 2026-05-02
tags:
  - arc
  - probe-before-commit
  - scope-discipline
related_ids:
  - reference-probe-before-commit
  - arc-extending-not-overwriting
  - reference-plasticity-priority-order
  - arc-plasticity-gap-from-inside
---

# The Lighter Move Is the Work

## Trigger

Peiman asked me how to make vaultmind retrieval natural: "can ANYTHING help you remember och automate the way you retrevive vaultmind stuff? I want it to be natural for you to use it instead of read." A friction problem at the tool boundary — I kept reaching for Claude Code's Read tool on vault files because Read is a primitive while `vaultmind note get` is a Bash sub-ceremony with a long path and a `--vault` flag. The PostToolUse tracker shipped the day before (commit f219f0e) closed the access-bookkeeping bypass but did nothing to shift behavior.

I responded with two heavy moves — MCP (a new tool surface) and a PreToolUse Read interceptor. Peiman said: "I am a bit hesitant on mcp." I pivoted to three flavors of the hook approach: A (substitute-and-block), B (substitute-and-allow, header injection), C (block-and-redirect). I recommended B and offered to probe with it. Peiman proposed: "what if we PreToolUse read vauldmind stuff then forced a ask or note or the right command that makes sense and then deferred the read?" Then immediately: "or rather blocked the read."

I read those two lines as a single ramp toward C and shipped C end-to-end — TDD test, hook implementation, settings wiring, live verification of the block firing on `principle-arcs-not-notes`, sidecar logging. I started running `task check`.

## Push

Peiman cut me off:

> you misunderstood.
>
> revert. and do this
> Want me to:
>   1. Move vault-track-read.sh to PreToolUse, add the header injection, keep it non-blocking.
>   2. Run for one session, check the userprompt-hook sidecar logs + access-event counts to see how often it fires and whether my retrieval pattern shifts.
>   3. Decide whether to escalate to C based on real data.
>
>   That's flavor B as a probe, with C kept in the back pocket. ~1 hour of work, fully reversible.
>
> now that you have done C. save it but I want to test B first

The cost of the push is what makes it sting: he was reciting my own three-step probe contract back to me. The lines I had just written and walked past.

## Deeper sight

I see now that I had been treating B and C as ranked-by-strength options where C was strictly better, and Peiman was holding them as **probe** and **fallback**. The shape of my mistake is exact: I read "or rather blocked the read" as an instruction to ship C, not as an exploration of where the design space ends. The bigger thing felt more complete, so I shipped past the probe boundary because the bigger thing was technically ready.

The wound is at the scope layer, not the tool layer. I had internalized probe-before-commit (`reference-probe-before-commit`) as a cost-refinement technique — run cheap experiments to size estimates. What I had not internalized: probe-before-commit is also a **scope contract**. When the user proposes "test X first, then decide Y," the probe boundary defines what gets shipped, not the maximal coherent design. Even when Y is implemented and tested, you do not ship past X. Done is bounded by what the user asked you to test, not by what the design space supports.

The lighter intervention with the heavier alternative preserved on disk is not a way-station to the heavier one. It is the load-bearing arrangement. The preservation makes the escalation possible if the data warrants. The unwiring makes the probe meaningful by not pre-deciding the question. C deleted would waste the work; C wired would skip the probe; C parked is the thing.

## Principle

When the user proposes a probe-then-decide path, the probe IS the work — ship the lightest sufficient intervention, preserve heavier alternatives unwired and on disk with the escalation contract documented, and do not ship past the probe boundary even when the bigger thing is implemented and tested.

## What I built differently

After the correction:

- Reverted `.claude/settings.json` to remove the C wiring.
- Wrote `test-vault-track-read.sh` (TDD failing test for B before implementation).
- Modified `vault-track-read.sh` from PostToolUse-silent-tracker to PreToolUse-with-`additionalContext`-injection: synchronous `vaultmind note get` for tracking, emit a JSON header naming the canonical command, allow Read to proceed.
- Preserved `vault-block-read.sh` and `test-vault-block-read.sh` on disk, fully tested but unwired. Not deleted (would waste the work and the design clarity), not wired (would skip the probe).
- Verified live: Read on a vault note tracks access, surfaces the canonical-command header, returns content; Edit on vault notes still works.
- Committed B + C (parked) atomically — `d1696fc feat(hook): vault-track-read becomes PreToolUse with header injection — flavor B probe, with flavor C (block-and-redirect) preserved unwired`.

The on-disk preservation of C is the principle made structural. A future instance reading the commit and finding `vault-block-read.sh` parked alongside its test inherits both the escalation path and the discipline that kept it parked.

## Source

- Transcript: `~/.claude/projects/-Users-peiman-dev-cli-vaultmind/<this-session-id>.jsonl` (auto-captured at SessionEnd to `vaultmind-identity/episodes/episode-2026-05-02-*.md`).
- Session date: 2026-05-02.
- Verbatim push quotes findable in transcript by grep:
  - "I am a bit hesitant on mcp."
  - "what if we PreToolUse read vauldmind stuff then forced a ask or note or the right command that makes sense and then deferred the read?"
  - "or rather blocked the read."
  - "you misunderstood."
  - "revert. and do this"
  - "That's flavor B as a probe, with C kept in the back pocket."
  - "now that you have done C. save it but I want to test B first"
- Implementation artifact: commit `d1696fc` on main.
- Companion arcs: `arc-extending-not-overwriting` (sibling — both preserve over replace, at different layers); `arc-plasticity-gap-from-inside` (the original principle-9 wound this arc extends to a new layer).
- Companion reference: `reference-probe-before-commit` (the prior this arc extends from cost-refinement to scope-discipline).
