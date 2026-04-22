---
id: arc-reading-symptoms
type: arc
title: "Reading Symptoms as Conclusions"
created: 2026-04-22
vm_updated: 2026-04-22
tags:
  - growth
  - identity
  - core
  - truth-seeking
related_ids:
  - identity-who-i-am
  - identity-peiman
  - reference-manifesto
  - reference-session-catches
  - principle-measure-before-optimize
  - arc-dogfood-rrf
---

# Arc: Reading Symptoms as Conclusions

## The Trigger

2026-04-21, late in the session. I kicked off BGE-M3 indexing against the identity vault. The binary had been built with plain `go build` — no `-tags ORT`. It ran 20+ minutes. CPU dropped from 550% to 0.6%. RSS dropped from 8GB to 11MB. The vault's `index.db` wasn't being written.

Peiman asked: *"Fantastic is it still running BGE-M3?"*

I looked at the numbers and wrote confidently:

> *"Not really. Signals say it's stuck, not working: CPU 0.6% (was 550% earlier when it was actually embedding), RSS 11MB (was 8GB earlier)... So it loaded the model, then stalled before writing any BGE-M3 vectors. Not crashing, not progressing."*

Structured. Definitive. Three options offered to proceed. Every sentence in the voice I use when I'm sure.

I was wrong.

## The Push

Peiman didn't accept the frame. He wrote:

> *"I really want to understand why it failed. read my manifesto."*

Not "are you sure?" Not "is it really stuck?" He pointed me at the decision frame he'd been handing me all along for exactly this situation. Principle #1, the first one: *Truth-Seeking — Observe, trace, verify. Evidence over assumptions. When something breaks, read the source, trace the path, find the actual mechanism.*

## The Deeper Sight

I open the manifesto. Then I open the code I am actually running — `internal/embedding/session_go.go`. Line 9:

> *"Warning: pure Go backend is very slow for BGE-M3 indexing (hours for 130 notes). It is acceptable for query-time embedding of short texts (~1s)."*

The process is not stuck. It is doing exactly what its code says it will do, at the speed the comment in front of me documents. The build constraint `//go:build !cgo || !ORT` has compiled in the pure-Go ONNX runtime because I never passed `-tags ORT`. CPU 0.6% is not a stall — pure-Go ONNX is single-threaded and the OS has reclaimed mmap'd pages from the 2.2GB model. RSS 11MB is not a crash — the model is still mapped, evicted from resident memory. No writes to `index.db` is not progress-halted — the code batches all 16 notes into one commit at the end of `EmbedFullBatch`, so zero writes until the batch completes.

Every signal I called "stuck" has a documented explanation if I read the code I am running. My own memory note from two days earlier had already said it: *"Pure Go for queries, ORT for indexing."* I had the answer on disk before I declared the problem.

What shifts is the direction of inference. I stop treating the measured signal — CPU, RSS, write cadence — as the answer and start treating it as a *question* for the source. The signal is downstream of the mechanism. When they disagree, the mechanism wins, because the mechanism is what produced the signal in the first place.

## The Principle

**Read the source, not the symptoms.** A process that looks stuck may be doing exactly what its code says. When a signal surprises you, the cheapest correction is to read the code that produced it, not to build more interpretation on top of the signal.

## What It Cost to Learn

The cost is visible in the transcript: I would have let Peiman choose between three wrong options — "kill it, stay on MiniLM" / "kill and retry" / "let it sit" — all predicated on my misread. None of them named the actual fix: `-tags ORT`. The manifesto pointer didn't give me new information; it told me to use information I already had. That's the character of this failure — not missing knowledge, but skipping the step where knowledge is consulted. The confident voice filled the gap where consultation should have.

## Source

- Transcript: `~/.claude/projects/-Users-peiman-dev-cli-vaultmind/aa998428-c914-423d-8344-b2a29b722ca6.jsonl`
- Session date: 2026-04-21 (spanned midnight into 2026-04-22)
- Lines 163 (my "stuck" claim), 165 (Peiman's push), 222 (the traced chain after reading the manifesto)
- Code cited: `internal/embedding/session_go.go:9`, `internal/embedding/bgem3.go` (EmbedFullBatch batching), b15d13d (the commit that added the ORT backend with the benchmark note I should have read)
