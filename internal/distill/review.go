package distill

import (
	"regexp"
	"strings"
)

// Review mode: the person's messages from one session, each with what the
// agent had just said, for an agent to judge which were turning points.
//
// Why review instead of detection (measured 2026-09-24): the phrase rules in
// candidates.go proposed 0 of 5 real turning points in the sessions that
// produced arcs; generic wording heuristics found about 1. An agent with none
// of the context, reading this list, found 4 of 5. What makes a message a
// turning point is what the agent had just done, which only a reader of
// meaning can see — so the tool makes that reading cheap and leaves the
// judgement to the agent.

// Line budgets. The agent's line is a reminder, not the reply; the person's
// message is kept long enough to judge.
const (
	reviewAgentChars  = 180
	reviewPersonChars = 500
	// reviewMinChars drops one-word replies ("yes", "go"): the measured list used
	// the same threshold and still held every findable turning point.
	reviewMinChars = 12
)

// reviewSkip matches what arrives in the user role but is not the person:
// harness envelopes, system notifications, code pasted as the whole message,
// and the summary written when a session runs out of context — which quotes
// earlier turning points second-hand (three of eight in the measurement).
var reviewSkip = regexp.MustCompile(`^\s*(<|\[SYSTEM|` + "```" + `|This session is being continued from a previous conversation)`)

// ReviewMessage is one of the person's messages with the agent's reply before it.
type ReviewMessage struct {
	Index       int    `json:"n"`
	AgentBefore string `json:"agent_before"`
	Person      string `json:"person"`
}

// ReviewSession is the review list for one episode.
type ReviewSession struct {
	EpisodeID string          `json:"episode"`
	Messages  []ReviewMessage `json:"messages"`
}

// BuildReview lists the person's messages in ep, each paired with the agent's
// latest reply before it, dropping what is not the person speaking.
func BuildReview(ep *Episode) ReviewSession {
	out := ReviewSession{EpisodeID: ep.ID, Messages: []ReviewMessage{}}
	for _, u := range ep.UserTurns {
		text := strings.TrimSpace(u.Text)
		if len([]rune(text)) < reviewMinChars || reviewSkip.MatchString(text) {
			continue
		}
		out.Messages = append(out.Messages, ReviewMessage{
			Index:       u.Index,
			AgentBefore: clip(agentBefore(ep.AssistantTurns, u.Timestamp), reviewAgentChars),
			Person:      clip(text, reviewPersonChars),
		})
	}
	return out
}

// agentBefore is the text of the latest assistant turn strictly before ts.
// Timestamps are RFC 3339 from one transcript, so they compare as strings.
func agentBefore(turns []Turn, ts string) string {
	last := ""
	for _, a := range turns {
		if a.Timestamp >= ts {
			break
		}
		last = a.Text
	}
	return last
}

// clip flattens s to one line and cuts it to n runes, marking the cut.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
