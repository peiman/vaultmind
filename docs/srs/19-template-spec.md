# Template Specification

> See also: [config spec](18-config-spec.md), [frontmatter schema](04-frontmatter-schema.md), [CLI reference](11-cli-reference.md)
>
> **New in v3** — this section was missing from v2.

## Overview

Templates are Markdown files that define the initial content for `note create`. Each type in the [type registry](18-config-spec.md) specifies a template path.

## Template Location

Template paths in config are relative to vault root.

Convention: `templates/<type>.md` (e.g., `templates/project.md`).

## Template Format

Templates are standard Markdown files with YAML frontmatter. Placeholder variables use `<%=variable%>` syntax. This delimiter avoids collision with both Obsidian Templater (`{{}}` and `${}`) and standard Markdown/HTML.

### Available Variables

| Variable | Value |
|----------|-------|
| `<%=id%>` | Generated or user-provided ID |
| `<%=type%>` | The note type |
| `<%=title%>` | Title from `--field title=...` or derived from path |
| `<%=created%>` | Current date in ISO 8601 |
| `<%=updated%>` | Current date in ISO 8601 |
| `<%=date%>` | Current date (`YYYY-MM-DD`) |
| `<%=datetime%>` | Current datetime (`YYYY-MM-DDTHH:MM:SS`) |
| `<%=path%>` | Vault-relative path of the new note |

### Example Template

```markdown
---
id: <%=id%>
type: <%=type%>
status: active
title: <%=title%>
created: <%=created%>
vm_updated: <%=vm_updated%>
tags: []
related_ids: []
---

## Overview



## Notes


```

## ID Generation

When `note create` is called without an explicit `--field id=...`:

1. If the type registry recommends a prefix (derived from type name), use `{type}-{slug}` where slug is the filename without extension, lowercased, spaces replaced with hyphens.
2. Example: `vaultmind note create projects/payment-retries.md --type project` generates `id: proj-payment-retries`.

The generated ID is checked for uniqueness against the index before creation.

## Template Processing Rules

1. All `<%=variable%>` placeholders are replaced. Unrecognized variables are left as-is (with a warning).
2. `--field key=value` flags override template frontmatter values.
3. `--body <text>` replaces the entire body section below the frontmatter.
4. Core fields (`id`, `type`, `created`, `vm_updated`) are always set, even if the template omits them.
5. The resulting file must pass [validation](13-validation-rules.md) before being written.

## Missing Template

If a type's template path doesn't resolve to an existing file:

- `note create` generates a minimal note with only frontmatter (core + required fields) and an empty body
- A warning is emitted
