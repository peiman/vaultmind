# Evidence for surfaced notes

Doctor measures observations, not whether an agent used a note or whether the
note influenced an answer. Each surfaced appearance has one state:

- **opened:** a `note_access` event with `source: note_get` in the same
  conversation and vault, after surfacing. `note get` and the existing
  `vault-track-read.sh` hook on vault-file Reads already log this event.
- **cited:** the exact, case-sensitive note ID appears in that turn's assistant
  text or tool-call input values. A title mention does not qualify, nor does an
  ID that is only a prefix of a different ID. Reads take precedence.
- **unknown:** neither observation is available. Unknown is never unused;
  applying an idea in other words is not measurable by this check.

```text
Memory use (7d): 5 surfaced → 5 delivered → 1 opened, 2 cited (2 unknown — not measurable from text)
gap-closing notes: 1 surfaced, 1 cited
```

`gap-closing notes` counts the same appearances, limited to notes a closed gap names in `closed_by`. A note that was opened is not also counted as cited. There is no count of notes written.

Counts describe surfaced appearances, not unique notes. The same note appearing
in two retrievals contributes two appearances. Missing historical body-delivery
fields do not establish that a body was withheld.

## Capture path

`capture-episode.sh` already resolves `transcript_path` from the hook payload and
invokes `vaultmind episode capture <transcript> --incremental`. Single-file
capture now calls `experiment.DB.RecordUseEvidence`, whether incremental or
full. The canonical Claude Code and Codex installation wires capture at
**SessionEnd**. An installation that also invokes capture at **Stop** gets
observations after each turn. No new hook is needed and the recall script,
prompt output, and retrieval path are unchanged. Directory bootstrap capture
continues to capture episodes without recording evidence.

Claude Code recall records the harness conversation ID in
`sessions.user_session_id`; the retrieval event carries the surfaced note IDs,
vault, timestamp, and an invocation-session ID. There is no existing explicit
turn field. Evidence capture reconstructs turns from real user-prompt records,
using their UUID (or transcript line number when absent). Hook execution can
precede the appended prompt record: the gap after the prior assistant reply is
assigned to the upcoming prompt. Citations stay in that turn; they describe
an observation without claiming the injection caused it. Queued mid-turn messages, tool results, injected reminders,
and compaction summaries do not create new turn boundaries.
Whole-second prompt-hook timestamps prefer a prompt appended in that second;
multiple prompts in the same second leave the turn unknown rather than
borrowing another reply's citations.

Only assistant `text` blocks, string content, and `tool_use.input` string values
are searched. Attachments, `system` records (`hook_success`,
`hookAdditionalContext`, `additionalContext`), user records, tool results,
thinking, call metadata, and sidechains are excluded. Merely carrying the
injected recall block therefore cannot create citation evidence.

Logged reads are matched by conversation, vault, exact ID, and time. Capture
uses the turn boundary when available; otherwise it uses the existing
30-minute session window. Doctor also recognizes reads in that session window
before a transcript has been captured. `opened` remains observable without a
citation scan. A missing/unsupported transcript, an unmappable turn, or
missing telemetry leaves citation evidence unknown. The citation parser
currently supports Claude Code JSONL; Codex episode capture still works, but
its rollout format is not classified for citation evidence in part 1.

Capture is best effort: evidence errors are debug-logged and do not block
writing an episode. Telemetry `off` supplies no experiment session and writes
no evidence. All processing is local; assistant output and tool inputs are
searched but are not copied into the evidence events.

## Local events and JSON compatibility

A `use_evidence` experiment event stores `note_id`, `evidence`,
`surfaced_event_id`, `turn_id`, and `user_session_id`, using the source
retrieval's invocation-session and vault. Its deterministic event ID makes
repeated and concurrent capture idempotent per `(surfaced_event_id, note_id)`.
A repeated scan can upgrade an observation as transcript/access data arrives.
Stale or partial captures cannot erase stronger evidence already observed:
`unknown` can become `cited` or `opened`, and `cited` can become `opened`.
Retrieval shadow variants are not counted as extra surfaced appearances;
hybrid and keyword retrievals both participate.

Doctor's `memory_use` JSON adds `opened`, `cited`, and `unknown`, including
inside `per_caller`. Existing `injections`, `bodies_delivered`, and `consumed`
fields remain. `consumed` retains the legacy unscoped read-within-30-minutes
measurement for compatibility; new consumers should use `opened`.
`opened + cited + unknown = injections`, with each appearance counted once.
No schema-version bump or field rename is required.

`memory_use` also carries `gap_closing_surfaced` and `gap_closing_cited`.
`gap_loop` carries `opened`, `closed`, `median_days_to_close`, and
`gap_closing_cited`. `median_days_to_close` is a fraction of a day (12 hours
is 0.5) and `null` when nothing has closed. `gap_closing_cited` is `null`
when there is no usage log or the cite query fails, and `0` only when measured
as zero. The cite window is the same machine-wide window as `memory_use`.
Both are additive on schema `v1`.

Part 2's sampled self-report is not included.
