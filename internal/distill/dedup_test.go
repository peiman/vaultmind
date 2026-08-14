package distill

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// De-duplication, not extraction, is the failure mode that kills trust in a
// propose-only surface. The 2026-05-31 distillation review measured it: two
// independent miners mis-tagged the SAME candidate to two DIFFERENT existing
// arcs, both wrong. Extraction precision was fine; attribution was not.
//
// So the split is deliberate and load-bearing: the tool automates the FINDING
// (which existing arcs does this resemble, and how strongly) and refuses the
// JUDGEMENT (is this already covered). A single covered/new verdict is exactly
// what the corpus showed cannot be automated — and a wrong "already covered"
// silently deletes a real transformation, which is unrecoverable.

type fakeFinder struct {
	byText map[string][]NearArc
	err    error
	calls  int
}

func (f *fakeFinder) NearestArcs(_ context.Context, text string, limit int) ([]NearArc, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := f.byText[text]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func TestAnnotateNearestArcs_AttachesNeighboursToDeskEntries(t *testing.T) {
	f := &fakeFinder{byText: map[string][]NearArc{
		"The stranger test": {
			{ID: "arc-the-agent-is-the-user", Title: "The Agent Is the User", Score: 0.81},
			{ID: "arc-dogfood-rrf", Title: "RRF Is Not Cosine", Score: 0.44},
		},
	}}
	r := Report{DeskPending: []DeskEntry{{ID: "journal-x", Title: "The stranger test"}}}

	got := AnnotateNearestArcs(context.Background(), r, f, 3)
	require.Len(t, got.DeskPending[0].NearestArcs, 2)
	assert.Equal(t, "arc-the-agent-is-the-user", got.DeskPending[0].NearestArcs[0].ID)
	assert.InDelta(t, 0.81, got.DeskPending[0].NearestArcs[0].Score, 0.001)
}

func TestAnnotateNearestArcs_AttachesNeighboursToCandidates(t *testing.T) {
	f := &fakeFinder{byText: map[string][]NearArc{
		"you decide": {{ID: "arc-the-one-who-uses-it-decides", Title: "The One Who Uses It Decides", Score: 0.77}},
	}}
	r := Report{Candidates: []Candidate{{Rule: RuleAuthorityGrant, Verbatim: "you decide"}}}

	got := AnnotateNearestArcs(context.Background(), r, f, 3)
	require.Len(t, got.Candidates[0].NearestArcs, 1)
	assert.Equal(t, "arc-the-one-who-uses-it-decides", got.Candidates[0].NearestArcs[0].ID)
}

// The desk entry's body carries the transformation; its title is a headline.
// Matching on the body when one is present is what makes the neighbours useful
// rather than a title-keyword coincidence.
func TestAnnotateNearestArcs_PrefersBodySnippetOverTitle(t *testing.T) {
	f := &fakeFinder{byText: map[string][]NearArc{
		"body text about retrieval triggers": {{ID: "arc-holding-an-arc-is-not-having-it", Score: 0.9}},
	}}
	r := Report{DeskPending: []DeskEntry{{
		ID: "journal-x", Title: "A title", Snippet: "body text about retrieval triggers",
	}}}

	got := AnnotateNearestArcs(context.Background(), r, f, 3)
	require.Len(t, got.DeskPending[0].NearestArcs, 1)
	assert.Equal(t, "arc-holding-an-arc-is-not-having-it", got.DeskPending[0].NearestArcs[0].ID)
}

// A finder failure must not lose the candidates: neighbours are an aid, and
// losing the proposals to protect the aid inverts their importance.
func TestAnnotateNearestArcs_FinderFailureKeepsTheReport(t *testing.T) {
	f := &fakeFinder{err: errors.New("index locked")}
	r := Report{
		DeskPending: []DeskEntry{{ID: "journal-x", Title: "T"}},
		Candidates:  []Candidate{{Verbatim: "you decide"}},
	}

	got := AnnotateNearestArcs(context.Background(), r, f, 3)
	assert.Len(t, got.DeskPending, 1, "entries survive a failed lookup")
	assert.Len(t, got.Candidates, 1)
	assert.Empty(t, got.DeskPending[0].NearestArcs)
	require.NotEmpty(t, got.ParseErrors, "the degradation is reported, not silent")
	assert.Contains(t, strings.Join(got.ParseErrors, " "), "index locked")
}

// A nil finder is the no-index case (the vault was never indexed). It must be a
// clean no-op, not a crash and not an error.
func TestAnnotateNearestArcs_NilFinderIsANoOp(t *testing.T) {
	r := Report{DeskPending: []DeskEntry{{ID: "journal-x", Title: "T"}}}
	got := AnnotateNearestArcs(context.Background(), r, nil, 3)
	assert.Empty(t, got.DeskPending[0].NearestArcs)
	assert.Empty(t, got.ParseErrors)
}

// An entry with nothing to match on must not burn a query.
func TestAnnotateNearestArcs_SkipsEmptyText(t *testing.T) {
	f := &fakeFinder{byText: map[string][]NearArc{}}
	r := Report{DeskPending: []DeskEntry{{ID: "journal-x"}}}

	AnnotateNearestArcs(context.Background(), r, f, 3)
	assert.Zero(t, f.calls, "no text, no lookup")
}

// The rendered report must show the neighbours AND refuse the verdict. If it
// ever prints "already covered", a wrong call silently deletes a real
// transformation — the one error in this pipeline that cannot be undone.
func TestFormatReport_ShowsNeighboursWithoutVerdict(t *testing.T) {
	var buf strings.Builder
	require.NoError(t, FormatReport(Report{
		DeskPending: []DeskEntry{{
			ID: "journal-x", Title: "T", Date: "2026-08-13",
			NearestArcs: []NearArc{
				{ID: "arc-a", Title: "Arc A", Score: 0.82},
				{ID: "arc-b", Title: "Arc B", Score: 0.31},
			},
		}},
	}, &buf))

	out := buf.String()
	assert.Contains(t, out, "arc-a")
	assert.Contains(t, out, "0.82", "the score is the evidence; a bare list invites a guess")
	assert.NotContains(t, strings.ToLower(out), "already covered")
	assert.NotContains(t, strings.ToLower(out), "duplicate of")
	assert.Contains(t, out, "yours to judge",
		"say plainly that the verdict is the reader's, since the tool deliberately withholds it")
}

// A neighbour with no id still has to render something the reader can act on.
func TestFormatReport_NeighbourFallsBackToTitle(t *testing.T) {
	var buf strings.Builder
	require.NoError(t, FormatReport(Report{
		DeskPending: []DeskEntry{{ID: "journal-x", Title: "T", Date: "2026-08-13",
			NearestArcs: []NearArc{{Title: "An Untitled-Id Arc", Score: 0.5}}}},
	}, &buf))
	assert.Contains(t, buf.String(), "An Untitled-Id Arc")
}

// Long bodies are cut on a rune boundary, so a multi-byte character at the
// limit can't be split into invalid UTF-8 and poison the embedding query.
func TestHeadOf_CutsOnRuneBoundary(t *testing.T) {
	assert.Equal(t, "abc", headOf("abc", 10), "short input is returned whole")
	assert.Equal(t, "abc", headOf("  abc  ", 10), "surrounding space is trimmed")

	multi := strings.Repeat("é", 50) // 2 bytes per rune
	got := headOf(multi, 10)
	assert.Equal(t, 10, len([]rune(got)))
	assert.True(t, utf8.ValidString(got), "must not split a rune")
}
