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

**Retrieval check.** After the draft is written, `note propose` re-runs the
original question through the same ranking `ask` uses (`query.Rank`), with the
same retrieval options and the default hit window (`app.ask.search_limit`).
One pass excludes drafts and records the rank of the closest existing note:
the patch target, or, for a new note, the best note that is not a draft. A
second pass includes drafts and records the rank of the draft itself. That
second rank answers whether accepting the draft would make the question find
this content. A patch is measured on the proposal note, which holds the
finding. The check writes `rank_before`, `rank_after`, and `rank_checked_at`
into the draft's frontmatter through the mutation writer. `0` means the note
was not in the window. It does not record an access and it does not record an
activation read. Keyword mode, when the embedder is unavailable, still runs
and is labeled. The text line and the additive `rank_check` object in the v1
envelope report both measurements. This ADR does not add `draft` to any type's
allowed-status list.

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
  comment skips the append and deletes the index row before the proposal file,
  so a cleanup that failed after the write does not copy the section again. If
  the file delete fails, the row is already gone; the retry finds the file,
  re-indexes it, sees the marker, and only finishes cleanup. If the file is
  already gone and the indexed row is a proposal, accept removes the row and
  says the proposal was already gone. Delivery omits only a `<!-- vaultmind:`
  comment and keeps every other HTML comment. `note get` still shows the
  marker. Search text and recall excerpts still drop every HTML comment. The
  target's other frontmatter is left as written (Principle 7:
  `internal/mutation` is the one frontmatter writer). `sources` are added only
  when the target's type allows that field. The write re-reads the file and
  refuses when those bytes differ from the ones just read: nothing is written
  and the proposal stays. After a write that lands, accept deletes the index
  row, then the proposal file, and re-indexes the target.
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

`index.DraftIDs` loads every note whose status is `draft` or whose type is
`gap`, once per operation. `DraftSet` is that one set (the type is
`NonKnowledge`; the old name stays so callers do not fork). `Visible` hides a
gap even when the caller opted into drafts, and hides a draft unless they did.
The same predicate is the SQL in `appendSearchFilters`: a gap is always out,
and a draft is out only while `IncludeDrafts` is false. The start node of a
traversal and the target of a pack are the id the caller named, and they are
not passed through `Visible`.

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
- `TestDraftIDs_IsTheStatusDraftSet` (`internal/index`) — the shared set holds `status: draft` and `type: gap`. `Visible(gap, true)` stays false. `Visible(draft, true)` stays true. An empty set hides nothing.

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
- `TestContract_NoteProposeJSON` (`cmd`) — `note propose --json` is the v1 envelope, and `rank_check` names the draft's rank. The check writes no `note_accesses` row.
- `TestContract_NoteAcceptJSON` (`cmd`) — `note accept --json` is the v1 envelope.
- `TestPropose_RankCheck_DraftRanksFirst` (`internal/propose`) — a draft that answers its question ranks first with drafts included, and the patch target's before-rank is the excluded-draft ranking. The frontmatter gains only `rank_before`, `rank_after`, and `rank_checked_at`. Computing the after rank with drafts excluded makes this test go red.
- `TestPropose_RankCheck_UnrelatedDraftDoesNotRank` (`internal/propose`) — a draft whose body does not answer the question is rank 0, and the line names the notes that outranked it.
- `TestPropose_RankCheck_KeywordModeStillRuns` (`internal/propose`) — with no embedder the check still runs and the line says keyword.
- `TestBandFor_WithheldJudgementIsEmpty` (`internal/query`) — a small vault with an unmeasured floor shows no band.

What stays on the honor system: not putting `draft` into a type's allowed-status
list. That list is vault data. A closed list warns during the draft window and
already rejects an unknown value at validation; accepting writes a value the
list allows. A type with an empty list is allowed to store `draft`.

## Amendment: knowledge gaps

A gap is a question the vault did not answer. It is a note (`type: gap`,
`status: open`, `question`, `opened_at`, optional `session`, `asked_hits`),
written under `gaps/` by `note gap`. The index is a regenerable cache, so the
committed note is the only durable home (Principle 7 — one source of truth).
Commands stay under `note` (ADR-101). `doctor` is where the open list and the
meter are read.

**Schema.** `gap` is registered the way `overview` is. When a vault already
defines types and does not define `gap`, `LoadConfig` adds it in memory and
does not rewrite `config.yaml`. A vault's own `gap` definition wins. A vault
with an empty types map stays empty: inventing a type there would make every
other note an unknown type. `note gap` then fails and names the fix,
`types.gap` in `.vaultmind/config.yaml` or `vaultmind init`. The built-in
definition has no template path, so an existing vault needs no file on disk.
Required fields are `title` and `question`. Statuses are `open` and `closed`,
never `draft`. `authoritative` is false: a gap is not citable evidence. New
vaults list the same definition in the scaffold. `index.TypeGap` and
`vault.GapType` are the same string; a vault test locks them because the two
packages cannot import each other.

**Dedup.** An open gap with the same question, spacing and case aside, is
reported (`mode: existing`). A closed gap does not count. The id is
`gap-<slug>` from the question. A path collision with a different question
gets a numeric suffix.

**Recall.** A gap is never knowledge. The draft exclusion is the same set and
the same SQL predicate, extended by one type clause (`gapExcludeSQL`).
`--include-drafts` does not reveal a gap. Named reads stay direct: `note get`
returns the gap, the full `tree` lists it and marks `[gap]`, and `tree --for`
omits it. `asked_hits` is whatever `ask` ranks for the question at open time,
with drafts and gaps already excluded.

**Closing.** `note propose --gap <id>` requires an open gap. The draft stores
`closes_gap`. An omitted `--question` uses the question on the gap. `note
accept` closes that gap before it changes the draft, through `internal/mutation`
(`status: closed`, `closed_at`, `closed_by`). `closed_by` is the new note, or
the note a patch lands on. Already closed is a no-op, so a retry finishes.
Frontmatter other than those keys stays byte-identical.

A confirmed absence is not a failure. The id is not in the index, so the
finding is still the knowledge, and leaving the draft unpublished would
strand it. Accept publishes the note, prints a warning that names the id,
and `--json` adds a `warnings` entry with code `gap_missing`, that same
message, and the id in `field`. Status becomes `warning`. `schema_version`
stays `v1`. That warning is not tagged `reindex_failed`.

Anything short of confirmed absence fails inside the same error envelope as
every other accept failure, and the draft stays a draft. A lookup error is
`gap_missing` (the gap might still be there). A note that is not a gap, a
path outside the vault, and a file that cannot be parsed are `gap_invalid`.
A file that cannot be read is `gap_unreadable`.

**Hints.** A weak or no-match `ask` from the CLI appends one JSON line to
per-user state, not to the vault. The path is `xdg.StateDir()` plus
`possible-gaps/<sha256 of vault.CanonicalKey>.jsonl`. That is the same state
directory episode cursors and the signer socket use (Principle 7 — one
resolver). The usage database stays in the data directory; this log is
disposable. `CanonicalKey` is the absolute, symlink-resolved vault path,
the same string calibration stores. There is no separate vault id. The
directory is `0700` and the file is `0600`. The file name is not salted.

The query hash is `sha256(salt || query)`, raw salt bytes then the query.
One 32-byte salt per install lives beside that directory, at
`possible-gap-salt`, mode `0600`, created on first use. One salt serves
every vault. The log is already one file per vault, and the salt's job is
to stop a dictionary of unsalted hashes on this machine. A salt per vault
would make the same question hash differently in two vaults and add a file
that has to stay with the log. A salt of the wrong length is an error. It
is not replaced.

`app.ask.gap_log_limit` (default 1000, the same number as
`gap.DefaultLogLimit`) is how many records a compaction keeps. The file may
grow to twice that. The next append rewrites it to the newest window, via a
temp file in the same directory and `rename`. The scan keeps at most twice
the window in memory. A non-positive value uses the default, so a missed
config read cannot empty the log. The key is config and
`VAULTMIND_APP_ASK_GAP_LOG_LIMIT`, not a flag.

The line stores `query_sha256` and `query_len` (bytes). The raw query is
stored only when `app.ask.gap_store_query` is true. The default is false.
The switch is config and `VAULTMIND_APP_ASK_GAP_STORE_QUERY`, not a flag.
Strong, moderate, and empty bands are not hints. A failed write does not
fail `ask`.

A hook caller is not a hint. `recordPossibleGap` returns before it writes
when `index.HookCaller` is true. `HookCaller` and the access-log resolver
share one classification: a caller whose name contains `hook`, and the
exact value `auto-rag-guard`. That value is not renamed, so the script
keeps writing it. New note-access rows from that caller are stored as
`hook`, because the resolver and `HookCaller` are the same function. Rows
already stored under the old caller string stay as they are.
`experiment.DetectCaller` still returns the raw value.

An old `.vaultmind/possible-gaps.jsonl` is not read and not migrated. It
never shipped in a release.

`doctor` prints the count and the earliest date, and `--json` adds `gaps`
without bumping `schema_version`. The meter is gaps opened (open plus
closed), gaps closed, and the median days to close. An unreadable log
counts as zero.

## Enforcement (gaps)

Removing the gap half of the shared predicate fails these tests. They do not
replace the draft list above. `--include-drafts` is set, and the gap is still
absent:

- `TestSearchFTS_ExcludesGapsEvenWhenDraftsAreIncluded` (`internal/index`) — FTS search and count. Deleting `gapExcludeSQL` lets the gap rank and this test goes red. The draft beside it still comes back only when drafts are included.
- `TestGapGraph_StaysOutOfRecallEvenWhenDraftsAreIncluded` (`cmd`) — a gap that is a real wikilink neighbor stays out of `ask`, `search`, `memory pack`, `memory context-pack`, `memory related`, `memory neighbors`, `links neighbors`, and MCP `ask`/`search`, with and without `--include-drafts`. `arc recite --type gap` totals 0. `tree --for` omits the covering gap. The full tree marks `[gap]`. `note get` still returns the body.
- `TestAccept_ClosesTheLinkedGap` (`internal/propose`) — accepting a new draft and accepting a patch each close the linked gap. `closed_by` is the new note, or the patched target. The gap file is byte-identical except `status`, `closed_at`, and `closed_by`. Skipping the `closes_gap` read leaves the gap open and this test goes red.
- `TestContract_NoteAcceptClosesTheLinkedGap` and `TestContract_NoteAcceptWarnsWhenTheLinkedGapIsMissing` (`cmd`) — a linked gap that is in the vault closes and the envelope stays `ok`. A linked gap that is not in the vault still accepts: the text warns, and `--json` carries `gap_missing` with the id in `field`. Status is `warning`.
- `TestContract_NoteAcceptRejectsALinkedNoteThatIsNotAGap` (`cmd`) and `TestAccept_LinkedNoteThatIsNotAGapStaysADraft` (`internal/propose`) — a `closes_gap` id that is not a gap returns `gap_invalid` and leaves the draft a draft. Dropping `failOp` on that path goes red.
- `TestAccept_UnreadableGapStaysADraft` and `TestAccept_UnparseableGapStaysADraft` (`internal/propose`) — a gap file that cannot be read (`gap_unreadable`) or parsed (`gap_invalid`) leaves the draft a draft.
- `TestOpen_WritesAGapAndDedups` (`internal/gap`) — the written note is `type: gap`, `status: open`, and a repeated question reports `existing` without ranking again.
- `TestContract_NoteGapJSON` (`cmd`) — `note gap --json` is the v1 envelope, and `doctor` lists the open gap. The human line and the `gaps` object agree.
- `TestRecordPossible_BandsAndPrivacy` (`internal/gap`) and `TestRecordPossibleGap_LogsWeakAsksOnly` (`cmd`) — weak and no-match are logged; strong, moderate, and empty are not. The raw query is absent unless `app.ask.gap_store_query` is set.
- `TestRecordPossible_LogStaysOutsideTheVault` (`internal/gap`) — the log path is outside the vault, the file is `0600`, the directory is `0700`, and an old `.vaultmind/possible-gaps.jsonl` is left unread. Writing the log inside the vault again makes this test red.
- `TestHashQuery_DependsOnTheInstallSalt` (`internal/gap`) — the same query hashes differently under two salts, a planted salt matches `sha256(salt || query)`, and the salt file is 32 bytes at mode `0600`. Dropping the salt makes the two hashes equal and this test red.
- `TestRecordPossible_OneSaltForEveryVault` (`internal/gap`) — two vaults share one salt, so the same query has one hash and two files.
- `TestRecordPossible_CompactsPastTwiceTheLimit` (`internal/gap`) — the file may hold `2N` records and the next one rewrites it to the newest `N`. A non-positive limit keeps the default window.
- `TestRecordPossibleGap_SkipsHookCallers` (`cmd`) — `vaultmind-userprompt-hook` and `auto-rag-guard` append nothing; a CLI caller appends one line. Dropping the `HookCaller` check makes this test red.
- `TestHookCaller_AutoRagGuardIsAHook` (`internal/index`) — `auto-rag-guard` is a hook without renaming `VAULTMIND_CALLER`.
- `TestAsk_AppendsAPossibleGapForATie` (`cmd`) — an `ask` whose band is weak or no-match appends a line that does not contain the raw query, and the line is not inside the vault.

## Amendment: credit the note that closed the gap

A note is gap-closing when a closed gap stores `closed_by` as that note id.
The credit follows that note id, not the gap id. `ask` loads the map once
from `frontmatter_kv` (the question, `opened_at`, `closed_at`). It does not
read the gap files at delivery time. When the text answer includes that
note's body, as the target or as a context item, one line follows it:

```text
  ↳ answers a question first investigated 2026-10-14: "<question>"
```

The date is `opened_at`, in UTC, cut to the calendar day. The question is
cut at 80 characters. `--pointers-only` does not print the line. A note
already shown in the session is not credited again, in the text or in
`answers_gap`. `--json` adds `answers_gap` on the target and on a context
item whose body was delivered, and on a hit with the same id. A draft or a
gap never carries it, even with `--include-drafts`. `schema_version` stays
`v1`.

`doctor` counts cites of those notes with the same rule as other cites: a
read wins, and an appearance that was not cited is only surfaced. The cite
window is the machine-wide window `memory_use` already uses (7 days,
`experiments.db`). `gap-closing notes (all vaults on this machine): N surfaced, M cited` follows the
memory-use line. `--json` puts gaps opened, gaps closed, median days to
close, and gap-closing notes cited on `gap_loop`. Median days is a fraction
of a day (12 hours is 0.5). `median_days_to_close` is null when nothing has
closed. `gap_closing_cited` is null when there is no usage log or the cite
query fails, and 0 only when the count was measured as zero. Writing a note
is not a score.

## Enforcement (credit)

- `TestGapClosures_MapsTheNoteThatClosedTheGap` (`internal/index`) — one query builds the map. An open gap, a non-gap, and a row that does not parse are absent. Two gaps naming one note keep the earlier `opened_at`, then the lower gap id.
- `TestAsk_DeliversTheGapCreditLine` (`internal/query`) — the line is under the target and the context item, absent for a normal note, absent for `--pointers-only`, and absent when the note was shown earlier. `answers_gap` is present on the gap-closing note and absent on the other, including when the note was already shown. Replacing the lookup with an empty map makes this test red.
- `TestAsk_ContextItemWithoutABodyHasNoAnswersGap` (`internal/query`) — a context item with `BodyIncluded: false` has no `answers_gap`. Dropping the body check makes this test red.
- `TestAsk_NonKnowledgeNotesCarryNoCredit` (`internal/query`) — a draft and a gap carry no credit in the text or in JSON, even when the draft would be delivered with `--include-drafts`. The line function itself skips a non-knowledge note.
- `TestAsk_LoadsTheGapCreditMapOnce` (`internal/query`) — the lookup runs once per command. A lookup error does not fail the answer.
- `TestGapClosingUse_CountsOnlyGapClosingCites` (`internal/experiment`) — a plain cite is not counted, and an opened gap-closing note is not a cite. An empty id list is zero. Returning an empty clause, so every cite counts, makes this test red.
- `TestGapClosingUse_CountsPastAThousandIDs` (`internal/experiment`) — ids past the first chunk are still counted.
- `TestAsk_CreditsTheNoteThatClosedAGap` (`cmd`) — accepting the draft that closes a gap, then asking, prints the line. `--pointers-only` does not. `--json` is `v1` and carries `answers_gap`. A hook caller still sees the line. The second ask in the same session does not repeat it.
- `TestDoctor_GapLoopReadsTheClaim` (`cmd`) — `doctor --json` `gap_loop` has `opened`, `closed`, `median_days_to_close`, and `gap_closing_cited`. The text line is `gap-closing notes (all vaults on this machine): N surfaced, M cited`.
- `TestDoctor_GapClosingCitedIsNullWithoutALog` (`cmd`) — with no usage log, `gap_closing_cited` is null. Reporting 0 instead makes this test red.
- `TestGapClosingCounts_QueryErrorIsNull` (`cmd`) — a failed cite query is null, and the failure is logged.

## References

- Gap loop, item 5 (2026-10-14): a delivered note that closed a gap names the question, and doctor counts cites of those notes.
- Gap loop, item 1 (2026-10-14): `note gap` records the question; accepting the draft that answers it closes the gap.
- Gap loop, item 4 (2026-10-14): `note propose` checks that the draft repairs retrieval.
- Gap loop, item 3 (2026-10-14): drafts at the final path; accepting is a status flip.
- Gap loop, item 2 (2026-10-14): `note propose` writes the draft; `note accept` promotes it.
- ADR-002 and ADR-005: one registry key, generated constants, no literals in commands.
- ADR-014: this section is the enforcement record.
- Principle 7 (one file, one key, one filter) and Principle 8 (ranking policy in the binary).

---

**Decision Date:** 2026-10-14
**Decision Makers:** VaultMind maintainer
