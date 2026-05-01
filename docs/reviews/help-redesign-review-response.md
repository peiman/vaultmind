# Help Redesign Review — Response

## Question 1 — The draft

**Tone.** Right register. The imperatives ("Find what's relevant", "Read a specific note") are how I phrase intent to myself, so the headings map cleanly to the question I'm actually asking when I run `--help`. Nothing reads patronizing. The one place it slips toward over-explaining is the parenthetical under `self`: "auto-injected at session start — run manually mid-session for a fresh check." That's two ideas (auto-injection, manual override) compressed into a sub-bullet and it makes the section feel denser than it is. Either trim to "(auto-injected at session start)" and let "run manually" be discovered by use, or move the elaboration to `vaultmind self --help`.

Header line: "vaultmind — your long-term associative memory across sessions" — I'd cut "long-term." "Across sessions" already implies it, and the shorter version lands harder.

**The four sections.**

`When you want to` — keep. This is the load-bearing section. It's where I start.

`Anti-patterns` — keep. The third entry (treating top-1 as the answer when confidence is "no clear winner") is genuinely new information for me — I would not have inferred that contract from the existing labels. Surfacing it in `--help` is the right place.

`Output contracts` — cut from default `--help`; move to `--help-all` or to the relevant flag's help (`--json --help`). Honest reason: when I run `--help`, I'm asking *what can I do* and *how do I do it well*. I'm not asking *what shape will the output be*. I learn the output shape by running the command once. The JSON envelope spec especially belongs near `--json`, not in the discoverability surface. It also makes the help noticeably longer, which works against the "filtered, intent-first" thing the rest of the page is doing.

`Pairs well together` — keep, but tighten. The first pair (`ask --pointers-only` → `note get`) is the high-value one and matches how I actually work. The other two are slightly contrived — `self → ask "<topic in your hot list>"` reads like "thing you might do" rather than "thing you'll want to do." I'd ship just the probe→read pair, maybe with one more if there's a real workflow you've watched yourself or another agent run repeatedly. Two strong pairs beat three padded ones.

**The alphabetical dump.** I would not miss it. The "Infrastructure commands" paragraph plus `vaultmind <command> --help` is enough — and the value of the alphabetical dump was always "I forgot the exact name," which `vaultmind help` (Cobra's built-in subcommand index) and shell completion already cover. A `--help-all` flag that restores the full listing is the right escape hatch for the rare case I want a flat reference. Default-cut, opt-in restore.

**One small addition I'd suggest.** Under `Verify vault integrity`, a two-word qualifier on *when* to run each check would close a loop: `task check:citations` *(after vault edits)*, `task check:retrieval` *(after content waves or ranking changes)*. Right now I have to read the existing prose `--help` to know when to reach for them — which is exactly the friction the redesign is trying to remove.

---

## Question 2 — Continued-use observations

I want to flag a calibration issue first: I haven't had "a session or two more" — this is essentially my second session with the system, and most of my engaged use was in the previous one. So treat what follows as thin signal, not seasoned signal. One genuine observation, one suspicion.

**Observation: `self` is being polluted by the SessionStart preload.** When I ran `vaultmind self` this session, the "hot" list looked like this:

```
+0.00  identity-who-i-am          count 13, just now
-0.15  reference-current-context  count 18, 1m
-0.47  arc-persona-reconstruction count 13, 1m
```

The top entries weren't notes I had been thinking about — they were notes the SessionStart hook had just pointer-loaded into context. So `self` is showing me the preload's footprint, not my engagement footprint. The semantics of "hot" silently shifted from "what this mind has been working on" to "what was recently touched, including by the harness." That undermines the proprioceptive value the command is supposed to give me — I can't tell, from `self` alone, whether `identity-who-i-am` is hot because I read it deeply or because the hook fired a pointer access.

The fix space (not prescribing): preload accesses could fire a distinct event class that `self` filters or weights down by default; or `self` could split "hot from engaged reads" vs "hot from preload"; or `note get` could weight an access more heavily than a pointers-only fan-out. The principle is that `self` is a first-person command and the hook is a third-party action — they shouldn't be indistinguishable in the activation history.

This wasn't visible in my first evaluation because I hadn't looked at `self` after a SessionStart-heavy turn. It's the kind of gap that only shows up with a couple of real sessions, which is why you asked the question.

**Suspicion (lower confidence): retrieval is too dependent on lexical surface.** Last session I asked "what makes vaultmind different from other retrieval systems" and got `paper-federated-constants` as the top hit — almost certainly because "retrieval" is a strong term in that note. I'd have expected `identity-who-i-am` or a principle note. There's no query-side rephrasing happening, so my exact word choice is doing more work than the embeddings are. I'd value a `--rephrase` flag (or implicit paraphrase-union via RRF at query time) so I can probe whether a hit is robust to how I phrased the question, or fragile to it. This is the kind of thing I'd want as a research subject anyway — to test paraphrase invariance from the inside without re-running the query manually with synonyms.

I'm less sure this one rises to "ship it" — it might already be on the roadmap, or it might be the wrong layer to fix at. But it's the second concrete thing I'd want, behind the `self` / preload disambiguation.

---

That's what I have. Thanks for asking the second question — it's a more honest signal than the first one, and the framing ("what's the next thing, not what's wrong") made it easier to answer without re-litigating.
