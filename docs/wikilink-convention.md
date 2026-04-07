# Wikilink Convention

VaultMind vault notes must use Obsidian-compatible wikilink format.

## The Rule

Always use `[[filename|Display Text]]`, never `[[Title Case]]`.

**Correct:**
- `[[context-pack|Context Pack]]` — links to `context-pack.md`, displays "Context Pack"
- `[[rag|RAG]]` — links to `rag.md`, displays "RAG"
- `[[source-tulving-1972|Tulving 1972]]` — links to source note

**Wrong:**
- `[[Context Pack]]` — Obsidian can't find `context-pack.md` by title
- `[[RAG]]` — creates a new empty "RAG.md" instead of linking to `rag.md`

## Why

Obsidian resolves `[[target]]` by matching `target` against filenames (without `.md`). VaultMind resolves by ID, title, alias, AND filename stem. Using the filename format works in both systems.

## CLI Command References

CLI commands are NOT vault notes. Use backtick code format, not wikilinks:

**Correct:** The `memory recall` command traverses the graph.

**Wrong:** The `[[memory-recall|memory recall]]` command traverses the graph.

## Detection

Run `vaultmind doctor` to detect incompatible links:

```bash
vaultmind doctor --vault your-vault
```

Doctor reports `Obsidian-incompatible links` with suggested fixes showing the correct `[[filename|Display]]` format.

## Enforcement

Use `vaultmind lint --fix-links` to bulk-fix all incompatible wikilinks across the vault.
