# How VaultMind works

- **Vault** — a directory of Markdown notes with Obsidian-compatible frontmatter, tracked in Git. You curate it; the agent reads it.
- **Index** — a derived SQLite index: full-text (FTS5), dense + sparse + ColBERT embeddings (BGE-M3), and a link/alias knowledge graph. Rebuilt with `vaultmind index`; never hand-edited.
- **Retrieval** — Reciprocal Rank Fusion over the lanes, with top-hit confidence measured against each vault's own noise floor (the strong/moderate/weak bands are provisional: so far fit on a handful of vaults). A hit brings along the notes it links to (wikilinks, `related_ids`), so recalling one memory surfaces its neighbours. Ranking does **not** yet learn from use: usage-weighted ranking exists as an experiment, but replayed against real usage it did not improve recall, so it is not applied to results.
- **Long documents** — an imported doc past ~8,000 tokens (BGE-M3 embeds its first 8,192) becomes an index note plus one note per section, so each part is embedded, found and delivered on its own.
- **Context packs** — `vaultmind ask` assembles ranked results into a token-budgeted block ready to drop into an agent's context.
- **Delivery** — the pack carries note *text*, not just titles. A note larger than the remaining budget would otherwise contribute **nothing** while still being counted, which on a vault whose median note exceeds a hook's budget is every query rather than an edge case:

  ```bash
  vaultmind ask "spreading activation" --vault <path> --budget 4000 --excerpt 120
  ```

  `--excerpt N` caps each note at N tokens instead of dropping it, preferring the note's **Principle** section where it has one — arcs run trigger → push → deeper sight → principle, so the opening is story setup and the rule sits several sections down. Injecting "the first paragraph" hands an agent the anecdote and withholds the lesson. Off by default (`0`); the recall hook uses 160.

  A weak-but-correct top hit in a tight, well-curated vault delivers its body too. Low contrast between hits is a property of a *focused* vault, not evidence the top hit is wrong. Genuinely off-topic hits still land at or below the noise floor and stay suppressed.

  The context header states what actually arrived, in numbers you can recount from the output below it — `9 notes, 9 delivered as excerpts (968 tok)`. **Changed in 0.7.0** from the old `N items` form: if you parse that line, update it.

Everything is `--json`-able for programmatic use; every command returns a stable envelope.

See also [embedding-backends.md](embedding-backends.md) for the backends and the dense-only vs. 4-way hybrid tradeoff.
