# Risks and Mitigations

> See also: [safety](14-safety-model.md), [git model](07-git-model.md), [decisions](decisions.md)

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Users rely on frontmatter wikilinks as canonical edges | Broken backlinks on rename | Standardize on stable IDs; derive edges from wikilinks but warn |
| Destructive rewrites of human content | Data loss, trust erosion | Strict mutation boundaries; hash-based conflict detection |
| Dataview syntax drift | Broken generated regions | Template approved snippets; lint; refuse on malformed markers |
| Filename renames break references | Graph inconsistency | Immutable ID resolution; alias/title fallback |
| Overly rigid schema | User adoption resistance | Minimal required core; extensible domain tier; `--allow-extra` escape hatch |
| Noisy YAML diffs | Poor Git usability | Preserve key order; minimize re-serialization |
| Agent edits in dirty repos | Hard-to-review history | Git policy matrix with configurable refuse/warn |
| Inferred links over-trusted by agents | Reasoning errors | Separate edge types; explicit confidence metadata |
| Concurrent human + agent edits | File corruption | Hash-based conflict detection; single-writer assumption |
| Alias collisions across notes | Silent misresolution | Ambiguity detection; always surface candidates |
| Short aliases causing false positive mentions | Noisy inferred edges | Minimum alias length (3 chars); word-boundary matching |
| Large vaults causing slow initial index | Poor first-run experience | Incremental indexing; progress reporting; exclude patterns |
| Obsidian auto-update plugins rewriting `updated` field | Hash conflicts refuse VaultMind writes; broken write-confirm loop | Document conflict; recommend plugin exclusion or distinct field name (`vm_updated`) |
| YAML 1.1 boolean coercion (`yes`/`no`/`true`/`false`) | Silent data corruption of status fields and aliases | Require YAML 1.2 strict mode parser (go-yaml v3 strict) |
| Template syntax collision with Obsidian Templater | Templater auto-processes VaultMind templates, corrupting placeholders | Use `${variable}` syntax instead of `{{variable}}` |
| Inline `#tags` in body text not indexed | Incomplete tag graph, inflated IDF specificity scores | Document as explicit boundary; consider body tag extraction in v2 |
