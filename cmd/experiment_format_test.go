package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formatGap is a boundary formatter: 60s, 3600s, 86400s are real off-by-one
// cliffs. The test encodes the *contract at each cliff*, not every input.
func TestFormatGap_BoundariesAndUnits(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"below-minute", 59, "59s"},
		{"minute-cliff", 60, "1m"},
		{"below-hour", 3599, "59m"},
		{"hour-cliff", 3600, "1h"},
		{"below-day", 86399, "23h"},
		{"day-cliff", 86400, "1d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatGap(tc.in))
		})
	}
}

// An empty summary should still render counts (zero is real information),
// but must not print header rows for sections that have nothing to show —
// a clean report is a usable report.
func TestFormatUsageSummary_SuppressesEmptySections(t *testing.T) {
	var buf bytes.Buffer
	s := &experiment.UsageSummary{TotalSessions: 3, RetrievalEventCount: 7, UniqueNotesRecalled: 5}
	require.NoError(t, formatUsageSummary(s, &buf))

	out := buf.String()
	assert.Contains(t, out, "Sessions: 3")
	assert.Contains(t, out, "Retrieval events: 7")
	assert.Contains(t, out, "Unique notes recalled: 5")
	assert.NotContains(t, out, "Session gaps", "empty gap stats must not produce a header")
	assert.NotContains(t, out, "Top recalled", "empty top-notes must not produce a header")
}

// When gap stats are present, the compact unit formatting (formatGap) flows
// through — verifies the human output layer uses compact units, not raw seconds.
func TestFormatUsageSummary_GapStatsUseCompactUnits(t *testing.T) {
	var buf bytes.Buffer
	s := &experiment.UsageSummary{
		TotalSessions: 2, RetrievalEventCount: 10, UniqueNotesRecalled: 4,
		GapStats: experiment.GapStats{Count: 2, MedianSeconds: 90, P90Seconds: 3600, MaxSeconds: 86400},
	}
	require.NoError(t, formatUsageSummary(s, &buf))

	out := buf.String()
	assert.Contains(t, out, "median 1m", "90s must render as 1m")
	assert.Contains(t, out, "p90 1h", "3600s must render as 1h")
	assert.Contains(t, out, "max 1d", "86400s must render as 1d")
	assert.NotContains(t, out, "90s", "raw seconds must not leak to human output")
}

// TopNotes must render in order and include the retrieval count + last-seen.
// The count is the primary value — losing it would make the list useless.
func TestFormatUsageSummary_TopNotesCarryCounts(t *testing.T) {
	var buf bytes.Buffer
	s := &experiment.UsageSummary{
		TotalSessions: 1,
		TopNotes: []experiment.NoteStat{
			{NoteID: "arc-a", RetrievalCountTotal: 5, LastRetrievedTs: "2026-04-20T10:00:00Z"},
			{NoteID: "arc-b", RetrievalCountTotal: 1, LastRetrievedTs: "2026-04-19T09:00:00Z"},
		},
	}
	require.NoError(t, formatUsageSummary(s, &buf))

	out := buf.String()
	assert.Contains(t, out, "arc-a")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "arc-b")
	assert.Less(t, strings.Index(out, "arc-a"), strings.Index(out, "arc-b"),
		"top notes must render in the order the summary produced them")
}

// callerLine encodes a small attribution invariant: unknown-caller rows must
// not emit a dangling "caller=" — that has actually bitten log parsers before.
func TestCallerLine_UnknownCallerRendersExplicitUnknown(t *testing.T) {
	assert.Equal(t, "caller=unknown", callerLine("", nil))
}

// With caller + user + host, operator is composed as user@host — not just
// user, not just host. Regression guard for the identity line.
func TestCallerLine_ComposesUserAtHost(t *testing.T) {
	got := callerLine("hook", map[string]any{"user": "peiman", "host": "box"})
	assert.Contains(t, got, "caller=hook")
	assert.Contains(t, got, "operator=peiman@box")
	assert.NotContains(t, got, "operator=peiman ")
	assert.NotContains(t, got, "operator=box ")
}

// Host-only meta should still render an operator field rather than being
// dropped — otherwise remote systems without a user attribution would be
// silently attribution-less.
func TestCallerLine_HostOnlyStillRendersOperator(t *testing.T) {
	got := callerLine("h", map[string]any{"host": "box"})
	assert.Contains(t, got, "operator=box")
}

// Project dir, when set, appears after operator.
func TestCallerLine_ProjectDirAppended(t *testing.T) {
	got := callerLine("h", map[string]any{"user": "u", "claude_project_dir": "/x/y"})
	assert.Contains(t, got, "project=/x/y")
	assert.Less(t, strings.Index(got, "operator="), strings.Index(got, "project="))
}

// formatSessionTrace must keep empty queries visible — a blank query is a
// real event and "(no query text)" is the documented placeholder.
func TestFormatSessionTrace_EmptyQueryPlaceholder(t *testing.T) {
	var buf bytes.Buffer
	tr := sessionTrace{
		SessionID: "s-1",
		Events: []experiment.RetrievalEventSummary{
			{Timestamp: "2026-04-20T10:00:00Z", EventType: "search", Query: ""},
		},
	}
	require.NoError(t, formatSessionTrace(&buf, tr))
	assert.Contains(t, buf.String(), "(no query text)")
}

// formatSessionTrace must render every hit — dropping hits silently would
// mislead someone reading a trace to debug a retrieval.
func TestFormatSessionTrace_RendersEveryHit(t *testing.T) {
	var buf bytes.Buffer
	tr := sessionTrace{
		SessionID: "s-2",
		Caller:    "hook",
		Meta:      map[string]any{"user": "u"},
		Events: []experiment.RetrievalEventSummary{
			{
				Timestamp: "2026-04-20T10:00:00Z",
				EventType: "ask",
				Query:     "what is X",
				Hits: []experiment.RetrievalEventHit{
					{Rank: 1, NoteID: "n-1"},
					{Rank: 2, NoteID: "n-2"},
					{Rank: 3, NoteID: "n-3"},
				},
			},
		},
	}
	require.NoError(t, formatSessionTrace(&buf, tr))
	out := buf.String()
	assert.Contains(t, out, "n-1")
	assert.Contains(t, out, "n-2")
	assert.Contains(t, out, "n-3")
	assert.Contains(t, out, "events: 1")
}

// formatNoteTrace encodes the per-note view — attribution must appear so
// downstream users can tell whether a recall came from a hook or a human.
func TestFormatNoteTrace_CarriesCallerAttribution(t *testing.T) {
	var buf bytes.Buffer
	nt := noteTrace{
		NoteID: "n-42",
		Hits: []noteTraceHit{
			{
				SessionHit: experiment.SessionHit{Rank: 3, EventType: "ask", SessionID: "s-1"},
				Caller:     "workhorse-hook",
			},
		},
	}
	require.NoError(t, formatNoteTrace(&buf, nt))
	out := buf.String()
	assert.Contains(t, out, "Note n-42")
	assert.Contains(t, out, "1 retrievals")
	assert.Contains(t, out, "rank 3")
	assert.Contains(t, out, "caller=workhorse-hook")
	assert.Contains(t, out, "s-1")
}

// formatTau has a specific contract: nil means "undefined" and must produce
// a stable "  nan" token — tools grep for this. Non-nil renders to 3 places.
func TestFormatTau_NilRendersStableNan(t *testing.T) {
	assert.Equal(t, "  nan", formatTau(nil))
	v := 0.1234
	assert.Equal(t, "0.123", formatTau(&v))
	neg := -0.5
	assert.Equal(t, "-0.500", formatTau(&neg))
}
