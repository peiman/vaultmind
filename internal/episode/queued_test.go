package episode_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/episode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A message the person types while the agent is still working is not a "user"
// record: Claude Code queues it and records it as an attachment of type
// queued_command. Measured 2026-09-24 over 32 real transcripts: 296 such
// records, none also present as a user record — so an episode that reads only
// user records loses every one of them. One was the turning point of an arc
// ("…I hope doctor works and we are measuring everything of importance").
//
// Not every queued record is the person: task-notification is the harness,
// origin "peer" is another agent's message, and "auto-continuation" is a goal
// the harness re-sends. Records from older Claude Code carry no origin at all;
// those are the person's (all 9 inspected were). A message with a pasted image
// is a list of blocks; its text blocks are what the person wrote.
const queuedTranscript = `{"type":"user","timestamp":"2026-09-24T10:00:00.000Z","sessionId":"queued-0001","cwd":"/home/test","message":{"role":"user","content":"start the migration"}}
{"type":"assistant","timestamp":"2026-09-24T10:00:05.000Z","sessionId":"queued-0001","message":{"role":"assistant","content":[{"type":"text","text":"Starting."}]}}
{"type":"queue-operation","operation":"enqueue","timestamp":"2026-09-24T10:00:06.000Z","sessionId":"queued-0001","content":"and I hope we measure what matters"}
{"type":"attachment","timestamp":"2026-09-24T10:00:07.000Z","sessionId":"queued-0001","attachment":{"type":"queued_command","prompt":"and I hope we measure what matters","commandMode":"prompt","origin":{"kind":"human"}}}
{"type":"attachment","timestamp":"2026-09-24T10:00:08.000Z","sessionId":"queued-0001","attachment":{"type":"queued_command","prompt":"<task-notification>\n<task-id>x</task-id>\n</task-notification>","commandMode":"task-notification"}}
{"type":"attachment","timestamp":"2026-09-24T10:00:09.000Z","sessionId":"queued-0001","attachment":{"type":"queued_command","prompt":"[from workhorse] ready when you are","commandMode":"prompt","origin":{"kind":"peer"}}}
{"type":"attachment","timestamp":"2026-09-24T10:00:10.000Z","sessionId":"queued-0001","attachment":{"type":"queued_command","prompt":"remember to use a red team","commandMode":"prompt"}}
{"type":"attachment","timestamp":"2026-09-24T10:00:11.000Z","sessionId":"queued-0001","attachment":{"type":"hook_success","content":"ok"}}
{"type":"attachment","timestamp":"2026-09-24T10:00:12.000Z","sessionId":"queued-0001","attachment":{"type":"queued_command","prompt":"Goal set: finish the work","commandMode":"prompt","origin":{"kind":"auto-continuation"}}}
{"type":"attachment","timestamp":"2026-09-24T10:00:13.000Z","sessionId":"queued-0001","attachment":{"type":"queued_command","prompt":[{"type":"text","text":"[Image #2] why is this offline?"},{"type":"image","source":{"type":"base64","data":"AAAA"}}],"commandMode":"prompt","origin":{"kind":"human"}}}
`

func TestParseTranscript_KeepsWhatThePersonTypedMidTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queued.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(queuedTranscript), 0o600))

	ep, err := episode.ParseTranscript(path)
	require.NoError(t, err)

	var said []string
	for _, m := range ep.UserMessages {
		said = append(said, m.Text)
	}
	assert.Equal(t, []string{
		"start the migration",
		"and I hope we measure what matters",
		"remember to use a red team",
		"[Image #2] why is this offline?",
	}, said, "the person's queued messages are kept, with a pasted image's text; harness, peer and goal messages are not")

	require.Len(t, ep.UserMessages, 4)
	assert.Equal(t, "2026-09-24T10:00:07.000Z", ep.UserMessages[1].Timestamp,
		"a queued message keeps the time it was recorded")
	for _, m := range ep.UserMessages {
		assert.False(t, strings.Contains(m.Text, "task-notification"))
	}
}
