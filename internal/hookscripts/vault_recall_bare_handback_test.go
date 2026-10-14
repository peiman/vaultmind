package hookscripts_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hook receives a subagent hand-back as the bare envelope: the opening
// tag, the report, and the matching closing tag. A transcript may draw a
// prefix and a suffix around that envelope; those are not in the prompt.
// The report is opaque. A "<" in it is text. Parsing the body rejects the
// report (an unclosed "<foo", a quoted harness tag) and the hook sends the
// whole hand-back to recall, which times out.
//
// testdata/bare-handback.txt is an anonymized copy of one such prompt.
// Letters were replaced. The shape and the "<" bytes were kept.

func bareHandbackFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/bare-handback.txt")
	require.NoError(t, err)
	text := string(raw)
	// These three are the "<" bytes in the report. The unclosed one is what
	// the tag parser rejects; the others are text the parser must not judge.
	require.Contains(t, text, "<80")
	require.Contains(t, text, "`<xxxx-xxxxxxxxxxxx-xxxxx>`")
	require.Contains(t, text, "`<xxx`")
	require.True(t, strings.HasPrefix(text, "<agent-message "))
	require.True(t, strings.HasSuffix(text, "</agent-message>"))
	return text
}

func bareHandbackBody(t *testing.T) string {
	t.Helper()
	text := bareHandbackFixture(t)
	open := strings.Index(text, ">")
	require.Greater(t, open, 0)
	body := text[open+1:]
	body, ok := strings.CutSuffix(body, "</agent-message>")
	require.True(t, ok)
	return body
}

func wrapBareEnvelope(tag, attrs, body string) string {
	open := "<" + tag
	if attrs != "" {
		open += " " + attrs
	}
	return open + ">" + body + "</" + tag + ">"
}

func TestRecallHook_BareHandbackFixtureSkipped(t *testing.T) {
	assertCrossSessionSkipped(t, bareHandbackFixture(t))
}

func TestRecallHook_BareHandbackOpaqueSkipped(t *testing.T) {
	body := bareHandbackBody(t)
	// "<", "<=", a backtick after "<", a quoted harness tag, and an
	// unclosed tag. Each is text. None of them is structure.
	nasty := "confidence <80\nbound <= 1\ncompare <` now\nquoted \"<task-notification>\"\nunclosed <foo"
	cases := map[string]string{
		"teammate fixture":      wrapBareEnvelope("teammate-message", `teammate_id="agent" color="blue"`, body),
		"cross-session fixture": wrapBareEnvelope("cross-session-message", `from="agent" from-name="worker"`, body),
		"nasty agent":           wrapBareEnvelope("agent-message", `from="agent-1"`, "\n"+nasty+"\n"),
		"nasty teammate":        wrapBareEnvelope("teammate-message", `teammate_id="agent"`, "\n"+nasty+"\n"),
		"nasty cross-session":   wrapBareEnvelope("cross-session-message", `from="agent"`, "\n"+nasty+"\n"),
		"whitespace and BOM":    " \t\ufeff\n" + bareHandbackFixture(t) + "\n\t ",
	}
	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			assertCrossSessionSkipped(t, prompt)
		})
	}
}

func TestRecallHook_BareHandbackNeverSkipped(t *testing.T) {
	fixture := bareHandbackFixture(t)
	question := crossSessionQuestion
	cases := map[string]struct{ prompt, needle string }{
		"prose before":           {question + "\n" + fixture, question},
		"prose after":            {fixture + "\n" + question, question},
		"closing tag then prose": {"<agent-message from=\"agent-1\">\nreport\n</agent-message>\n" + question, question},
		"mentions the tag":       {"how does spreading activation work with an <agent-message> tag in the sentence", question},
		"mismatched close":       {"<agent-message from=\"agent-1\">\nreport on spreading activation\n</teammate-message>", "spreading activation"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := askQueryReceived(t, tc.prompt)
			assert.Contains(t, got, tc.needle, "prose outside a bare envelope must still be recalled")
		})
	}
}
