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
Accepting a new draft is flipping that field (`vaultmind note accept <id>`, or
`vaultmind frontmatter set <id> status accepted`). Same file, no copy
(Principle 7 — one source of truth). A proposal draft is accepted by
`vaultmind note accept` as well; that path is the amendment below.

**Drafts stay out of recall and context delivery by default** (Principle 8 —
the policy lives in the retrieval path, not in each command or hook).
`--include-drafts` (`app.retrieval.include_drafts`, default false) opts in.
The flag is registered on `ask`, `search`, `memory pack`, `memory related`,
`memory neighbors`, `arc recite`, `tree`, and the aliases `memory context-pack`,
`memory recall`, and `links neighbors`. Every one of them reads the one key
through `cmd/retrieval.go`. MCP ask and search shell out to those commands,
and federation uses the same ask path, so they agree without a second key.

`index.DraftIDs` is the one set of draft ids (Principle 7). Ranking's
`noteFilter` uses it. Graph and context delivery use it where candidates are
collected: the depth-1 context pack, graph traversal (a draft is dropped
before it is visited, so the walk does not pass through it), `memory related`,
`arc recite` (before the total and the omitted list — omitted means the budget
ran out, not "this was a draft"), and `tree --for`. A deeper context pack uses
the same traversal. The note the caller named stays: `note get`, and `memory
pack`, `memory related`, and `memory neighbors` of that id. `ask --read` only
reads a hit that ranked, so a draft that did not rank is not read that way.
`memory links` lists edge ids and titles, including a draft, and does not
deliver the body. The "shown earlier" path only strips bodies of notes already
in the pack. It does not load further notes.

`note get` still reads a draft, and its text header shows the status. The full
`tree` prints `[draft]` on that note's line. `tree --for` injects a one-line
body as context for a file, so a draft is left out of that list unless the
flag is set. `doctor` lists drafts awaiting review
(count, id, path, age) and `.md` files git reports as untracked or modified
whose working-tree mtime is older than `app.doctor.uncommitted_days` (default
7, `--uncommitted-days`). A file exactly that many days old is not flagged. A
vault that is not a git repository skips the uncommitted check and says why.
Neither list is an error. Doctor JSON stays `schema_version` `v1`; the new
fields are additive.

**Out of scope.** A before/after rank preview is a later slice. This ADR does
not add `draft` to any type's allowed-status list.

## Amendment: a proposal draft, and `note accept`

An accepted note cannot hold an unreviewed addition. Recall would return that
paragraph: the draft exclusion applies to a note whose `status` is `draft`, not
to a section inside a note that already ranks (Principle 8). Until a person
reviews it, the addition is its own draft. After they accept it, one copy
remains (Principle 7).

`vaultmind note propose --question --finding` retrieves on the question and the
finding the way `ask` does, drafts excluded, and reads the relevance band `ask`
already computed for the top hit. It does not invent a score. The band that
means close is the registry key `app.notepropose.close_band` (flag
`--close-band`). The default is the strong band. Only a strong match is close
enough to attach a finding to an existing note. A moderate, weak, `no_match`,
or empty band — including a judgement `ask` withholds when the vault is small
and the noise floor was never measured — is a new note. A typo'd close band
fails the same way: nothing matches it, so the finding becomes a new note
rather than a patch on the wrong one.

- **patch.** The top hit is close. The draft sits next to that note, at
  `<target-dir>/<target-id>.proposal-<yyyymmdd-hhmm>.md`. Frontmatter is
  `status: draft`, `patches: <target-id>`, `question:`, `sources:`, and the
  target's `type`. The body is the finding plus a Sources section.
- **new.** Nothing is close. Frontmatter is `status: draft`, `question:`, and
  `sources:`. The type is the top hit's type when that type is registered,
  otherwise the vault's default knowledge type: the registered type named
  `concept` when the schema has it, otherwise the first type in alphabetical
  order that can hold `draft`. The built-in `overview` type is skipped. The
  file is in the top hit's folder, or the vault root when there is no hit. The
  id is the id `note create` would mint for a filename slugged from the
  question. The body is expanded through that type's template, the same path
  `note create` uses, so required fields hold.
- **existing.** A draft already stores the same `question:` once spacing is
  collapsed and case is folded. That draft is reported. Nothing new is written.

`vaultmind note accept <id>` is how a person promotes a draft.

- A new draft (no `patches` value) flips `status` to the schema's published
  value. When the type lists statuses, that value is `accepted` if the list
  contains it, otherwise `active` if the list contains that. A list that
  contains neither is an error; the command does not guess `proposed`. An empty
  list publishes as `accepted`. Same file, same id.
- A patch draft is the draft whose `patches` field is non-empty. There is no
  second mode field. Accept appends the proposal body under `## <question>` on
  the target. The line immediately before that heading is the comment
  `<!-- vaultmind:accepted <proposal-id> -->`. A retry that already finds the
  comment skips the append and only deletes the proposal file and its index
  row, so a cleanup that failed after the write does not copy the section
  again. Search text and recall excerpts drop HTML comments, so the marker is
  not recalled. The target's other frontmatter is left as written (Principle 7:
  `internal/mutation` is the one frontmatter writer). `sources` are added only
  when the target's type allows that field. The write re-reads the file and
  refuses when those bytes differ from the ones just read: nothing is written
  and the proposal stays. After a write that lands, accept deletes the proposal
  file and re-indexes the target.
- A note that is not a draft is refused. A patch whose target is gone is refused.
  A patch whose target is itself a draft, or is itself a proposal, is refused.
  A `file:` source is checked the same way as a path: it must exist, and it
  must sit inside the vault or the repository.

The notes a write touched are re-indexed the way `note create` re-indexes, and
embedded when embed-on-write is on. `vaultmind frontmatter set` still changes
one field. It does not merge a proposal.

A type with a closed status list does not gain `draft`. During the draft window
validation warns that the status is not in the list, the same warning `note
create` already allows a write to pass. Accepting writes a value the list allows.

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
- Ask, search, the context pack, related notes, neighbors, arc recital, `tree --for`, MCP, serve and federation share one exclusion.
- Doctor shows what is waiting, without turning that waiting into a failure.

### Negative

- A note that used `status: draft` for some other meaning disappears from ranking. No vault in this repo used that value. Types with a closed status list reject `draft` at validation time.
- `app.retrieval.include_drafts` is not under a command prefix, so `--include-drafts` is registered by hand. One key is the point.

### Neutral

- `note get` and the full `tree` show drafts. They do not rank them. `tree --for` does not inject a draft's line unless `--include-drafts` is set. `memory links` lists a draft's id and title.
- The uncommitted check uses go-git and the working-tree mtime. It does not shell out, and it ignores non-markdown paths. Measured on a vault inside a large repository, the check alone stayed under the 1s bar for switching to a native `git status`.

## Implementation Notes

`index.SearchFilters.ExcludesDrafts` is the policy (true unless `IncludeDrafts`
is set). `appendSearchFilters` applies it in SQL for FTS, before `ORDER BY` /
`LIMIT`, with `IFNULL` so a missing status stays searchable. `noteFilter`
applies it for the in-memory lanes (embedding, sparse, ColBERT, activation)
before sort and limit. Commands only set `IncludeDrafts`. They do not drop hits
themselves.

The key is read in `cmd/retrieval.go` only. That file owns `app.retrieval` for
the namespace check, the same way write helpers own `app.embed_on_write`. Each
command above registers `--include-drafts` and calls that function, so a config
file and `VAULTMIND_APP_RETRIEVAL_INCLUDE_DRAFTS` reach all of them. Flag beats
env, env beats the file, the file beats the default. A deprecated alias does
not copy a hand-registered flag, so each alias registers it too.

`index.DraftIDs` loads `SELECT id FROM notes WHERE status = ?` once per
operation. `DraftSet.Visible` is the check. Zero `IncludeDrafts` excludes, so
existing library callers stay safe. The start node of a traversal and the
target of a pack are the id the caller named, and they are not passed through
`Visible`.

## Enforcement

Removing the draft exclusion fails these tests. They build a note that would
rank first and assert it is absent unless the caller opted in:

- `TestSearchFTS_ExcludesDraftsUnlessAsked` (`internal/index`) — FTS search and count, before the limit.
- `TestSparseRetriever_DropsADraftThatWouldMatch` (`internal/query`) — an in-memory lane through `noteFilter`.
- `TestDraftNotes_StayOutOfRecallUntilAsked` (`cmd`) — `ask` and `search`, default off, `--include-drafts` on.
- `TestDraftNotes_FederatedAskOmitsTheDraft` (`cmd`) — federated ask.
- `TestDraftNotes_MCPAskAndSearchOmitTheDraft` (`cmd`) — MCP ask and search.
- `TestDraftGraph_ContextOmitsTheNeighborUnlessAsked` (`cmd`) — a draft that is a real neighbor, by wikilink, shared tag, and alias mention, stays out of `ask` context, `memory pack`, `memory context-pack`, `memory related`, `memory neighbors`, and `links neighbors`, and comes back with `--include-drafts`. Removing `withoutDraftCandidates` makes the context assertion fail.
- `TestDraftGraph_ReciteSkipsADraftArc` (`cmd`) — `arc recite` skips a `status: draft` arc unless `--include-drafts` is set.
- `TestDraftGraph_NamedDraftStaysReadable` (`cmd`) — `memory pack` of a named draft id still returns its body. `ask --read` of a draft that did not rank does not.
- `TestDraftGraph_TreeForSkipsADraftLine` (`cmd`) — `tree --for` omits a draft's line unless `--include-drafts` is set. The full tree still lists the draft and marks `[draft]`.
- `TestDraftGraph_MCPAskOmitsTheNeighbor` (`cmd`) — MCP ask does not deliver the neighbor's body.
- `TestDraftIDs_IsTheStatusDraftSet` (`internal/index`) — the shared set is exactly `status: draft`.

`TestCheckUncommittedNotes_FlagsOnlyOldUntrackedMarkdown` fails if the age
comparison stops being strictly greater than N days. `TestDoctor_ListsDraftsAndSkipsUncommittedOutsideGit`
fails if a non-git vault is reported as a git failure instead of a skip.
`TestCheckUncommittedNotes_LimitsToAVaultInsideARepo` fails if a vault that is
a subdirectory of a repository reports files outside that vault.
`TestCheckUncommittedNotes_FollowsAGitfile` fails if a vault whose `.git` is a
file (a gitlink) is treated as not a repository.
`TestRender_MarksADraftAndLeavesOtherStatusesAlone` fails if `tree` drops the
`[draft]` mark or stamps it on another status.

`note propose` and `note accept` are held by these tests:

- `TestPropose_PatchWhenTheTopHitIsClose` (`internal/propose`) — a strong top hit writes a proposal draft next to that note.
- `TestPropose_NewWhenNothingIsClose` (`internal/propose`) — a weaker band, and no hit, write a new draft. Inverting the band check (`band == closeBand`) turns the weaker hit into a patch and this test goes red.
- `TestPropose_ExistingDraftOnRepeatedQuestion` (`internal/propose`) — the same question, spacing and case aside, reports the draft already written.
- `TestPropose_WrittenNoteValidatesAndAskOmitsDrafts` (`cmd`) — the written note passes the vault schema, and a default `ask` on the real index does not return the draft.
- `TestAccept_NewFlipsStatus` (`internal/propose`) — accepting a new draft flips status to the schema's published value.
- `TestAccept_PatchMergesDeletesAndReindexes` (`internal/propose`) — the target body contains the addition, the proposal file is gone, and the target is re-indexed. Skipping the proposal delete leaves the file and this test goes red.
- `TestAccept_RetryAfterCleanupFailureDoesNotDuplicate` (`internal/propose`) — when deleting the proposal fails after the target was written, a retry leaves one copy of the section. Removing the marker check appends it again and this test goes red.
- `TestAccept_FrontmatterPreservesUnchangedKeys` (`internal/propose`) — accepting a new draft changes only the `status` line. Key order, a quoted title, a date scalar, and a comment stay byte-identical.
- `TestAccept_PatchKeepsFrontmatterExceptSources` (`internal/propose`) — accepting a patch keeps that same frontmatter and appends `sources` when the type allows the field.
- `TestAccept_RefusesWhenTheTargetChanged` (`internal/propose`) — a target that changed between the read and the write is left untouched. Dropping the hash check overwrites it and this test goes red.
- `TestAccept_WriteUsesATempFile` (`internal/propose`) — the write goes through a temp file in the note's directory. A directory that cannot create one leaves the note unchanged.
- `TestAccept_RefusesADraftTarget` (`internal/propose`) — a patch whose target is a draft is refused and left unchanged. Allowing a draft target makes this test go red.
- `TestAccept_RefusesAProposalTarget` (`internal/propose`) — a patch whose target is itself a proposal is refused.
- `TestAccept_NonDraftErrors` (`internal/propose`) — accepting a note that is not a draft fails.
- `TestPropose_SourceValidation` (`internal/propose`) — a source URL must parse, and a path must exist inside the vault or the repository. `file:///etc/passwd` is rejected with the same error as `/etc/passwd`.
- `TestSourceURL_FileScheme` (`internal/propose`) — a `file:` URL of a note inside the vault is accepted; an empty `file:` URL is not.
- `TestStripForFTS_RemovesHTMLComments` (`internal/parser`) — the indexed text omits the accept marker, including a comment that contains `>`.
- `TestExcerpt_DropsTheAcceptMarker` (`internal/memory`) — a recall excerpt omits the marker when it sits on the same line as the sentence it guards.
- `TestContract_NoteProposeJSON` (`cmd`) — `note propose --json` is the v1 envelope.
- `TestContract_NoteAcceptJSON` (`cmd`) — `note accept --json` is the v1 envelope.

What stays on the honor system: not putting `draft` into a type's allowed-status
list. That list is vault data. A closed list warns during the draft window and
already rejects an unknown value at validation; accepting writes a value the
list allows. A type with an empty list is allowed to store `draft`.

## References

- Gap loop, item 3 (2026-10-14): drafts at the final path; accepting is a status flip.
- Gap loop, item 2 (2026-10-14): `note propose` writes the draft; `note accept` promotes it.
- ADR-002 and ADR-005: one registry key, generated constants, no literals in commands.
- ADR-014: this section is the enforcement record.
- Principle 7 (one file, one key, one filter) and Principle 8 (ranking policy in the binary).

---

**Decision Date:** 2026-10-14
**Decision Makers:** VaultMind maintainer
