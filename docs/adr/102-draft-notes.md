# ADR-102: Draft Notes Stay Out of Recall Until Accepted

## Status

Accepted

## Context

A note that is not ready should be writable in the vault without showing up in
recall. Copying it to a staging path and moving it on accept would make two
files for one note, and every command that ranks (`ask`, `search`, the MCP
tools, `serve`, federation) would need its own idea of which file counts.

`status` already records a note's lifecycle (`active`, `paused`, `accepted`,
and the rest). The value `draft` was unused. Types whose schema lists allowed
statuses keep that list; a type with an empty list, such as `concept`, accepts
any value. A draft is exactly the string `draft`. Nothing else about `status`
changes.

## Decision

**A draft is a note at its final path whose frontmatter `status` is `draft`.**
Accepting it is flipping that field (`vaultmind frontmatter set <id> status
accepted`, or any value other than `draft`). Same file, no copy (Principle 7 —
one source of truth).

**Drafts are excluded from ranking by default** (Principle 8 — the policy lives
in the retrieval path, not in each command or hook). `--include-drafts`
(`app.retrieval.include_drafts`, default false) opts in. The flag is registered
on `ask` and `search`; both read the one key. MCP and `serve` run those
commands, and federation uses the same retriever, so they agree without a
second filter.

`note get` still reads a draft, and its text header shows the status. `tree`
prints `[draft]` on that note's line. `doctor` lists drafts awaiting review
(count, id, path, age) and `.md` files git reports as untracked or modified
whose working-tree mtime is older than `app.doctor.uncommitted_days` (default
7, `--uncommitted-days`). A file exactly that many days old is not flagged. A
vault that is not a git repository skips the uncommitted check and says why.
Neither list is an error. Doctor JSON stays `schema_version` `v1`; the new
fields are additive.

**Out of scope.** `note propose` and a before/after rank preview are a later
slice. This ADR does not add `draft` to any type's allowed-status list.

## Alternatives Considered

### Alternative 1: A draft directory, copied into place on accept

**Pros:**
- Recall can ignore a whole directory without reading frontmatter.

**Cons:**
- Accepting copies or moves the file, so links and the id's path change.
- Two files can exist for one note.

**Why not chosen:** Accepting must be a status flip of the one file (Principle 7).

### Alternative 2: Filter drafts inside each command

**Pros:**
- Each command's tests would show the exclusion locally.

**Cons:**
- CLI, MCP, `serve` and federation would drift the moment one call site forgot the check.
- A filter after `LIMIT` would hide the top hit and return a short page.

**Why not chosen:** One retrieval policy, applied before the page is cut (Principle 8).

### Alternative 3: A new frontmatter field, `draft: true`

**Pros:**
- Leaves every existing `status` value untouched by construction.

**Cons:**
- A second field for the same fact. `status` already exists and `draft` was free.

**Why not chosen:** Principle 7. Drafts are the value `draft` and nothing else.

## Consequences

### Positive

- A draft is written where it will live. Accepting does not rename or copy it.
- Ask, search, MCP, serve and federation share one exclusion.
- Doctor shows what is waiting, without turning that waiting into a failure.

### Negative

- A note that used `status: draft` for some other meaning disappears from ranking. No vault in this repo used that value. Types with a closed status list reject `draft` at validation time.
- `app.retrieval.include_drafts` is not under the `app.ask` or `app.search` prefix, so the flag is registered by hand on both commands. One key is the point.

### Neutral

- `note get` and `tree` show drafts. They do not rank them.
- The uncommitted check uses go-git and the working-tree mtime. It does not shell out, and it ignores non-markdown paths.

## Implementation Notes

`index.SearchFilters.ExcludesDrafts` is the policy (true unless `IncludeDrafts`
is set). `appendSearchFilters` applies it in SQL for FTS, before `ORDER BY` /
`LIMIT`, with `IFNULL` so a missing status stays searchable. `noteFilter`
applies it for the in-memory lanes (embedding, sparse, ColBERT, activation)
before sort and limit. Commands only set `IncludeDrafts`. They do not drop hits
themselves.

The key is read in `cmd/retrieval.go` only. That file owns `app.retrieval` for
the namespace check, the same way write helpers own `app.embed_on_write`. Ask
and search register `--include-drafts` and call that function, so a config file
and `VAULTMIND_APP_RETRIEVAL_INCLUDE_DRAFTS` reach both. Flag beats env, env
beats the file, the file beats the default.

## Enforcement

Removing the draft exclusion fails these tests. They build a note that would
rank first and assert it is absent unless the caller opted in:

- `TestSearchFTS_ExcludesDraftsUnlessAsked` (`internal/index`) — FTS search and count, before the limit.
- `TestSparseRetriever_DropsADraftThatWouldMatch` (`internal/query`) — an in-memory lane through `noteFilter`.
- `TestDraftNotes_StayOutOfRecallUntilAsked` (`cmd`) — `ask` and `search`, default off, `--include-drafts` on.
- `TestDraftNotes_FederatedAskOmitsTheDraft` (`cmd`) — federated ask.
- `TestDraftNotes_MCPAskAndSearchOmitTheDraft` (`cmd`) — MCP ask and search.

`TestCheckUncommittedNotes_FlagsOnlyOldUntrackedMarkdown` fails if the age
comparison stops being strictly greater than N days. `TestDoctor_ListsDraftsAndSkipsUncommittedOutsideGit`
fails if a non-git vault is reported as a git failure instead of a skip.
`TestRender_MarksADraftAndLeavesOtherStatusesAlone` fails if `tree` drops the
`[draft]` mark or stamps it on another status.

What stays on the honor system: not putting `draft` into a type's allowed-status
list. That list is vault data, and a closed list already rejects unknown values
at validation. A type with an empty list is allowed to store `draft`.

## References

- Gap loop, item 3 (2026-10-14): drafts at the final path; accepting is a status flip.
- ADR-002 and ADR-005: one registry key, generated constants, no literals in commands.
- ADR-014: this section is the enforcement record.
- Principle 7 (one file, one key, one filter) and Principle 8 (ranking policy in the binary).

---

**Decision Date:** 2026-10-14
**Decision Makers:** VaultMind maintainer
