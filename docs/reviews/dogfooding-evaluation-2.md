# VaultMind Dogfooding Evaluation 2 — Agent Self-Assessment

**Date:** 2026-04-07
**Activity:** Used VaultMind CLI as an AI agent during real development work — writing design docs, researching theoretical foundations, answering questions about the codebase.
**Vault size:** 123 domain notes, 52 concepts, 40 sources

---

## Does VaultMind Make My Life Easier?

### What Works

**`memory summarize` is genuinely useful.** When I need to write a design doc or PR description and want the theoretical backing, `summarize` assembles the relevant notes in one call. I gave it 3 note IDs and got coherent excerpts about spreading activation, base-level activation, and ACT-R. I could use this to write a grounded PR description in 30 seconds instead of reading 3 full notes.

**`search` finds relevant content fast.** "graph traversal associative retrieval structured" immediately surfaced `concept-associative-memory`. For ad-hoc questions, this is faster than grepping the vault.

**`note get` on decision notes is high-value.** The `decision-structured-over-embeddings` note gave me the exact rationale with trade-offs — I could paste it into a design discussion as-is.

**`doctor` catches real problems.** We proved this during dogfooding sprint 1 — it found every broken link and incompatible wikilink. 3 bugs were discovered exclusively through real use.

### What Doesn't Work Well

**`context-pack` at large scale is frontmatter-heavy.** With 121 context items at depth 2, almost the entire budget goes to frontmatter. Only 1 note got its body included. For a 6000-token budget, I'd rather have 5 notes with full bodies than 121 notes with frontmatter only. The priority ordering works but the budget allocation needs a `--max-items` cap or a "body-first" mode.

**`memory recall` at depth 1 returns too many nodes.** Spreading Activation at depth 1 returned 82 nodes — nearly the whole vault. At 123 notes, almost everything is within 1 hop via `tag_overlap`. The inferred edges make the graph too dense for recall to be selective. Need either confidence filtering (`--min-confidence high`) or fewer `tag_overlap` edges.

**No way to ask "what do I know about X?" in one command.** I had to chain search → recall → summarize manually. A compound query like `vaultmind ask "optimal RAG context size"` that searches, gathers context, and presents a synthesis would be the killer feature.

**I can't use VaultMind during conversation without leaving the chat.** I have to build the binary, run CLI commands, and parse JSON output. If VaultMind were an MCP server or a Claude Code hook, I could query it transparently mid-conversation.

### Verdict

**VaultMind makes research assembly easier but not yet seamless.** The individual commands work. The knowledge base is valuable. But the workflow is still: think → manually query → read output → think. The gap is between "tool I use" and "memory I have." Closing that gap requires either MCP integration (transparent querying) or a compound command that does search+context+summarize in one shot.

---

## Specific Issues to Fix

| # | Issue | Impact | Suggested Fix |
|---|-------|--------|---------------|
| 1 | context-pack returns 121 frontmatter-only items instead of 5 with bodies | Agent gets breadth without depth — useless for reasoning | Add `--max-items` flag; body-first packing mode |
| 2 | recall at depth 1 returns 82/123 nodes via tag_overlap | Graph is too dense — recall is not selective | Raise tag_overlap threshold, or default to `--min-confidence medium` |
| 3 | No compound "ask" command | Agent must chain 3 commands manually | `vaultmind ask <query>` = search + context-pack + format |
| 4 | CLI-only — no in-conversation integration | Must exit conversation to query | MCP server wrapping VaultMind CLI |
| 5 | context-pack frontmatter is verbose | Aliases, tags, dates consume tokens without adding value for the query | Slim frontmatter mode: only include type, title, and relevant fields |
