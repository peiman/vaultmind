package distill

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rule 2 — recurrence → structural.
//
// The 2026-05-31 review called this "the proven money rule; the one thing a
// per-episode reader cannot do", and it was the only high-value rule left
// unbuilt. Its shape: one failure reappearing across MANY sessions, where the
// principle is always "this is structural, not a discipline problem". The
// review's own example recurred across 5+ distinct episodes — and no per-episode
// scan can see it, because each instance looks unremarkable alone.
//
// What is mechanised here is only the COUNTING: cluster proposals that talk
// about the same thing, and report the ones spanning several sources. The
// interpretation — what the recurrence means, whether it clears the arc bar —
// stays with the mind, exactly as the review concluded.

type fakeVectorizer struct {
	byText map[string][]float32
	err    error
}

func (f fakeVectorizer) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, ok := f.byText[t]
		if !ok {
			v = []float32{0, 0, 1} // unrelated to everything else in these tests
		}
		out[i] = v
	}
	return out, nil
}

// Three proposals about the same thing, from three different sessions, are the
// signal. One proposal repeated within a single session is not — that is one
// moment mentioned twice, and calling it structural would be a false positive.
func TestFindRecurrences_GroupsAcrossDistinctSources(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"use your vaultmind":      {1, 0, 0},
		"did you use your vaults": {0.98, 0.05, 0},
		"dont forget vaultmind":   {0.97, 0.08, 0},
	}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "use your vaultmind"},
		{Source: "episode-b", Text: "did you use your vaults"},
		{Source: "episode-c", Text: "dont forget vaultmind"},
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, 3, groups[0].SourceCount)
	assert.Len(t, groups[0].Members, 3)
}

func TestFindRecurrences_IgnoresRepeatsWithinOneSource(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"a": {1, 0, 0}, "b": {0.99, 0.01, 0},
	}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "a"},
		{Source: "episode-a", Text: "b"},
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	assert.Empty(t, groups,
		"twice in one session is one moment mentioned twice, not a pattern across time")
}

// Unrelated proposals must not be swept together. A recurrence report that
// groups everything says nothing.
func TestFindRecurrences_LeavesUnrelatedItemsAlone(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"about retrieval": {1, 0, 0},
		"about licensing": {0, 1, 0},
	}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "about retrieval"},
		{Source: "episode-b", Text: "about licensing"},
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	assert.Empty(t, groups)
}

// minSources is the dial between "noise" and "pattern". Raising it must be able
// to silence a group that a lower setting reports.
func TestFindRecurrences_HonoursMinSources(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"x": {1, 0, 0}, "y": {0.99, 0.01, 0},
	}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "x"},
		{Source: "episode-b", Text: "y"},
	}

	two, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	require.Len(t, two, 1)

	three, err := FindRecurrences(context.Background(), items, vz, 3)
	require.NoError(t, err)
	assert.Empty(t, three)
}

// Biggest span first: a shape recurring across five sessions is a stronger
// structural claim than one across two, and should be read first.
func TestFindRecurrences_OrdersByBreadth(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"p1": {1, 0, 0}, "p2": {0.99, 0, 0}, "p3": {0.98, 0, 0},
		"q1": {0, 1, 0}, "q2": {0, 0.99, 0},
	}}
	items := []RecurrenceItem{
		{Source: "e1", Text: "q1"}, {Source: "e2", Text: "q2"},
		{Source: "e3", Text: "p1"}, {Source: "e4", Text: "p2"}, {Source: "e5", Text: "p3"},
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, 3, groups[0].SourceCount, "the broader recurrence leads")
}

func TestFindRecurrences_DegenerateInputs(t *testing.T) {
	vz := fakeVectorizer{}
	for name, items := range map[string][]RecurrenceItem{
		"none": nil,
		"one":  {{Source: "a", Text: "x"}},
	} {
		t.Run(name, func(t *testing.T) {
			groups, err := FindRecurrences(context.Background(), items, vz, 2)
			require.NoError(t, err)
			assert.Empty(t, groups)
		})
	}

	t.Run("nil vectorizer", func(t *testing.T) {
		groups, err := FindRecurrences(context.Background(),
			[]RecurrenceItem{{Source: "a", Text: "x"}, {Source: "b", Text: "y"}}, nil, 2)
		require.NoError(t, err)
		assert.Empty(t, groups, "no embedder means no recurrence detection, not a failure")
	})
}

func TestFindRecurrences_EmbedFailureIsReported(t *testing.T) {
	_, err := FindRecurrences(context.Background(),
		[]RecurrenceItem{{Source: "a", Text: "x"}, {Source: "b", Text: "y"}},
		fakeVectorizer{err: errors.New("model down")}, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model down")
}

// The report must state what a recurrence MEANS, since the whole value of this
// rule is the reading — that a thing recurring across sessions is structural
// rather than a discipline failure — and must still refuse to draft the arc.
func TestFormatReport_RecurrenceSectionExplainsTheSignal(t *testing.T) {
	var buf strings.Builder
	require.NoError(t, FormatReport(Report{
		Recurrences: []RecurrenceGroup{{
			SourceCount: 3,
			Sources:     []string{"episode-a", "episode-b", "journal-c"},
			Members:     []string{"use your vaultmind", "did you use your vaults", "dont forget"},
		}},
	}, &buf))

	out := buf.String()
	assert.Contains(t, out, "3 sources")
	assert.Contains(t, out, "episode-a")
	assert.Contains(t, strings.ToLower(out), "structural",
		"the reading is the point: recurring across sessions means structural, not a discipline problem")
}

// The same message captured under two episode ids — which a compaction summary
// replaying an earlier turn produces routinely — is one moment counted twice,
// not a shape crossing sessions. The first real run reported exactly that.
func TestFindRecurrences_IdenticalTextIsNotRecurrence(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{"same push": {1, 0, 0}}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "same push"},
		{Source: "episode-b", Text: "same  push"}, // whitespace-only difference
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	assert.Empty(t, groups, "identical text is one message counted twice")
}

// A cluster held together by the phrase that surfaced it is the detector
// finding its own lexeme, not a recurring transformation. The first real run
// reported two of these — the partner saying "manifesto lens on" across
// sessions, which is true and is not a shift in anyone.
func TestFindRecurrences_DropsGroupsExplainedByASharedTrigger(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"do what you think, manifesto lens on": {1, 0, 0},
		"manifesto lens on, your call":         {0.99, 0.01, 0},
	}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "do what you think, manifesto lens on", Trigger: "manifesto lens"},
		{Source: "episode-b", Text: "manifesto lens on, your call", Trigger: "manifesto lens"},
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	assert.Empty(t, groups, "same trigger across sessions is the lexeme, not a transformation")
}

// Different triggers converging on the same shape is the real signal and must
// survive — that is a pattern recurring despite different surface phrasing.
func TestFindRecurrences_KeepsGroupsSpanningDifferentTriggers(t *testing.T) {
	vz := fakeVectorizer{byText: map[string][]float32{
		"you decide this one": {1, 0, 0},
		"if you are sure, go": {0.99, 0.01, 0},
	}}
	items := []RecurrenceItem{
		{Source: "episode-a", Text: "you decide this one", Trigger: "you decide"},
		{Source: "episode-b", Text: "if you are sure, go", Trigger: "if you are sure"},
	}

	groups, err := FindRecurrences(context.Background(), items, vz, 2)
	require.NoError(t, err)
	assert.Len(t, groups, 1)
}
