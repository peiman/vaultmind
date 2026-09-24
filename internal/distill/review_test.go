package distill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Measured 2026-09-24 (private experiments/2026-09-24-arc-candidates-recall):
// the phrase rules proposed 0 of 5 real turning points; an agent reading a
// compact list of the person's messages, each with what the agent had just
// said, found 4 of 5. A turning point is recognised by meaning — so the tool's
// job is to make that review cheap, not to guess.

func reviewEpisode() *Episode {
	return &Episode{
		ID: "episode-2026-09-24-abcd1234",
		UserTurns: []Turn{
			{Index: 1, Timestamp: "2026-09-24T09:00:00Z", Text: "please add retries to the fetcher"},
			{Index: 2, Timestamp: "2026-09-24T09:05:00Z", Text: "did you use your vaults? you have 2"},
			{Index: 3, Timestamp: "2026-09-24T09:06:00Z", Text: "<task-notification>\n<task-id>x</task-id>\n</task-notification>"},
			{Index: 4, Timestamp: "2026-09-24T09:07:00Z", Text: "This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion."},
			{Index: 5, Timestamp: "2026-09-24T09:08:00Z", Text: "yes"},
			{Index: 6, Timestamp: "2026-09-24T09:09:00Z", Text: "[SYSTEM NOTIFICATION - NOT USER INPUT] a background task finished"},
		},
		AssistantTurns: []Turn{
			{Index: 1, Timestamp: "2026-09-24T09:01:00Z", Text: "Added retries.\nThree attempts, capped backoff."},
			{Index: 2, Timestamp: "2026-09-24T09:04:00Z", Text: "I answered from what the session start gave me."},
		},
	}
}

func TestReview_KeepsWhatThePersonSaidWithWhatTheAgentHadJustSaid(t *testing.T) {
	r := BuildReview(reviewEpisode())
	require.Len(t, r.Messages, 2)
	assert.Equal(t, 1, r.Messages[0].Index)
	assert.Equal(t, "", r.Messages[0].AgentBefore, "nothing before the first message")
	assert.Equal(t, 2, r.Messages[1].Index)
	assert.Equal(t, "did you use your vaults? you have 2", r.Messages[1].Person)
	assert.Equal(t, "I answered from what the session start gave me.", r.Messages[1].AgentBefore,
		"the agent's latest reply BEFORE the person spoke")
}

// Machine text, compaction summaries (they quote turning points second-hand —
// three of eight in the measurement) and one-word replies are not moments.
func TestReview_DropsMachineTextCompactionSummariesAndOneWordReplies(t *testing.T) {
	r := BuildReview(reviewEpisode())
	require.Len(t, r.Messages, 2, "exactly the two real messages survive")
	for _, m := range r.Messages {
		assert.NotContains(t, m.Person, "task-notification")
		assert.NotContains(t, m.Person, "being continued from a previous conversation")
		assert.NotContains(t, m.Person, "SYSTEM NOTIFICATION")
		assert.NotEqual(t, "yes", m.Person)
	}
}

func TestReview_KeepsLinesShort(t *testing.T) {
	ep := reviewEpisode()
	ep.AssistantTurns[1].Text = strings.Repeat("long agent reply ", 100)
	ep.UserTurns[1].Text = strings.Repeat("a long message from the person ", 60)
	r := BuildReview(ep)
	assert.LessOrEqual(t, len([]rune(r.Messages[1].AgentBefore)), reviewAgentChars+1)
	assert.LessOrEqual(t, len([]rune(r.Messages[1].Person)), reviewPersonChars+1)
	assert.NotContains(t, r.Messages[1].AgentBefore, "\n")
}
