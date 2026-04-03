# Config Specification

> See also: [frontmatter schema](04-frontmatter-schema.md), [git model](07-git-model.md), [template spec](19-template-spec.md)
>
> **New in v3** — this section was missing from v2.

## Config File Location

Discovery order:

1. `--config <path>` flag (explicit)
2. `.vaultmind/config.yaml` in vault root
3. Built-in defaults

The vault root is determined by:

1. `--vault <path>` flag (explicit)
2. Current working directory

If no config file is found, VaultMind uses built-in defaults and emits a warning.

## Full Config Schema

```yaml
# .vaultmind/config.yaml

# Vault scanning
vault:
  exclude:
    - ".git"
    - ".obsidian"
    - ".trash"
    - "node_modules"
    - "_templates"        # customizable

# Type registry
types:
  project:
    required: [status, title]
    optional: [owner_id, due, area, tags, aliases, related_ids, source_ids]
    statuses: [active, paused, completed, cancelled]
    template: templates/project.md
  person:
    required: [title]
    optional: [aliases, tags, related_ids, url]
    statuses: []
    template: templates/person.md
  concept:
    required: [title]
    optional: [aliases, tags, related_ids, source_ids]
    statuses: []
    template: templates/concept.md
  source:
    required: [title, url]
    optional: [aliases, tags, related_ids]
    statuses: []
    template: templates/source.md
  decision:
    required: [title, status]
    optional: [related_ids, source_ids, tags]
    statuses: [proposed, accepted, rejected, superseded]
    template: templates/decision.md
  meeting:
    required: [title]
    optional: [tags, related_ids, source_ids]
    statuses: []
    template: templates/meeting.md

# Git integration
git:
  policy:
    dirty_unrelated: warn       # warn | refuse | allow
    dirty_target: refuse        # warn | refuse | allow
    detached_head: warn         # warn | refuse | allow
    merge_in_progress: refuse   # warn | refuse | allow
    no_repo: warn               # warn | refuse | allow

# Indexing
index:
  db_path: .vaultmind/index.db   # SQLite database location (relative to vault root)

# Memory engine
memory:
  alias_min_length: 3            # minimum alias length for mention detection
  tag_overlap_threshold: 1.0     # minimum score for tag_overlap edges
  context_pack_default_budget: 4096  # default token budget
```

## Config Validation

On load, VaultMind validates:

- All `types` entries have at least an empty `required` list
- All `statuses` are non-empty strings
- Template paths resolve to existing files (warning if not)
- Git policy values are one of `warn`, `refuse`, `allow`
- No duplicate type names

Invalid config is a fatal error — VaultMind refuses to start.

## Extending Types

To add a new note type, add it to the `types` section. The type name becomes a valid value for the `type` frontmatter field. No code changes needed.
