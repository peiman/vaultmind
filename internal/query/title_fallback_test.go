package query_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFuzzyTitleMatches_ReturnsTitlesSharingQueryTokens(t *testing.T) {
	titles := []index.NoteTitle{
		{ID: "a", Title: "The Judgment Gap"},
		{ID: "b", Title: "Depth over Speed"},
		{ID: "c", Title: "Becoming an Architect"},
	}

	got := query.FuzzyTitleMatches("judgment call", titles, 3)

	require.NotEmpty(t, got)
	assert.Equal(t, "a", got[0].ID, "the title containing 'judgment' should win")
}

func TestFuzzyTitleMatches_ExcludesTitlesWithNoTokenOverlap(t *testing.T) {
	// Workhorse's actual failing query. 'mislead' and 'myself' don't appear
	// in any title, so nothing should be suggested — better silent than
	// suggesting unrelated titles.
	titles := []index.NoteTitle{
		{ID: "a", Title: "The Judgment Gap"},
		{ID: "b", Title: "Depth over Speed"},
	}

	got := query.FuzzyTitleMatches("how do I mislead myself", titles, 3)
	assert.Empty(t, got, "no lexical overlap → no suggestion beats a bad suggestion")
}

func TestFuzzyTitleMatches_IgnoresShortStopwords(t *testing.T) {
	// Query contains only short words plus 'arc'. Shouldn't match titles by
	// happenstance on 'do', 'it', 'a' etc.
	titles := []index.NoteTitle{
		{ID: "a", Title: "A thing about it"}, // would match on naive char overlap
		{ID: "b", Title: "Arc of an architect"},
	}

	got := query.FuzzyTitleMatches("do it arc", titles, 3)
	require.NotEmpty(t, got)
	assert.Equal(t, "b", got[0].ID, "should match on 'arc', ignoring short tokens")
}

func TestFuzzyTitleMatches_LimitsToN(t *testing.T) {
	titles := []index.NoteTitle{
		{ID: "a", Title: "Judgment one"},
		{ID: "b", Title: "Judgment two"},
		{ID: "c", Title: "Judgment three"},
		{ID: "d", Title: "Judgment four"},
	}

	got := query.FuzzyTitleMatches("judgment", titles, 2)
	assert.Len(t, got, 2)
}

func TestFuzzyTitleMatches_CaseInsensitive(t *testing.T) {
	titles := []index.NoteTitle{
		{ID: "a", Title: "The JUDGMENT Gap"},
	}

	got := query.FuzzyTitleMatches("judgment", titles, 3)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

func TestWriteTitleSuggestions_RendersCallableHints(t *testing.T) {
	matches := []index.NoteTitle{
		{ID: "arc-judgment", Title: "The Judgment Gap"},
		{ID: "arc-depth", Title: "Depth over Speed"},
	}
	var buf bytes.Buffer

	wrote := query.WriteTitleSuggestions(&buf, matches)

	assert.True(t, wrote)
	out := buf.String()
	assert.Contains(t, out, "arc-judgment")
	assert.Contains(t, out, "The Judgment Gap")
	assert.Contains(t, out, "vaultmind note get", "each suggestion should show the command to read it")
	assert.True(t, strings.HasSuffix(out, "\n"))
}

func TestWriteTitleSuggestions_SilentOnEmpty(t *testing.T) {
	var buf bytes.Buffer
	wrote := query.WriteTitleSuggestions(&buf, nil)
	assert.False(t, wrote)
	assert.Empty(t, buf.String())
}
