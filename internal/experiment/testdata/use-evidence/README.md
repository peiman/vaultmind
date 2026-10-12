# Claude Code use-evidence fixture

`turns.jsonl` represents two turns of `evidence-session`. The test inserts
matching retrieval and access events at the fixture timestamps. In turn one:

| Surfaced ID | Evidence | Fixture observation |
| --- | --- | --- |
| `note-opened` | opened | Same-conversation, same-vault `note_get` event |
| `note-cited` | cited | Exact ID in assistant text |
| `note-injected` | unknown | ID only in hook attachment/system/additionalContext and tool-result records |
| `note-title` | unknown | Its title, **The Governing Principle**, appears in assistant text; its ID does not |
| `note-tool` | cited | Exact ID in an assistant Bash tool's command input |
| `note-string` | cited | Exact ID in assistant string content |
| `note-prefix` | unknown | Assistant mentions `note-prefix-extra`, a different ID |
| `note-later` | unknown | Exact ID appears only in the second turn |

The remaining negative cases put IDs in user text, tool results, thinking,
call metadata, sidechains, a different session's assistant output, or reads
from another conversation/vault. An `ask` access is not a read.

The title exists only to test the rejected title heuristic; the implementation
never reads or matches titles. No transcript text is persisted in evidence
rows.
