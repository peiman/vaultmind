package query

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cappedHeaderResult() *AskResult {
	return &AskResult{
		Query: "one two three four",
		TopHits: []retrieval.ScoredResult{
			{ID: "n", Title: "N", Score: 1},
		},
		QueryCapped:       &QueryCapped{EmbeddedWords: 512, TotalWords: 912, SalientTerms: 64},
		NoiseFloorApplied: true,
		TopHitConfidence:  ConfidenceStrong,
		TopHitCosine:      0.8,
		NoiseFloor:        0.2,
		NoiseFloorSigma:   0.1,
		RelevanceZ:        6,
		Vault:             "desk",
		Federated: []FederatedHit{{
			ScoredResult: retrieval.ScoredResult{ID: "n", Title: "N", Score: 1},
			Vault:        "desk",
		}},
		FederatedVaults: []FederatedVaultStatus{{Name: "desk", Contributed: true}},
	}
}

func TestWriteAskHeader_QueryCappedLineSitsUnderSearch(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, cappedHeaderResult(), true, true))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	assert.True(t, strings.HasPrefix(lines[0], "Search:"), "first line is the Search header")
	assert.Equal(t, "  [long query: ranked on its first 512 of 912 words; ColBERT and FTS on its 64 most salient terms]", lines[1])
	assert.Contains(t, lines[2], "relevance math:")
	assert.Contains(t, buf.String(), "[federated:")
}

func TestWriteAskHeader_LongQueryLineNamesOnlyWhatApplied(t *testing.T) {
	forward := cappedHeaderResult()
	forward.QueryCapped = &QueryCapped{EmbeddedWords: 512, TotalWords: 912, SalientTerms: 0}
	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, forward, false, false))
	assert.Contains(t, buf.String(), "  [long query: ranked on its first 512 of 912 words]\n")
	assert.NotContains(t, buf.String(), "salient")

	salient := cappedHeaderResult()
	salient.QueryCapped = &QueryCapped{EmbeddedWords: 100, TotalWords: 100, SalientTerms: 64}
	buf.Reset()
	require.NoError(t, writeAskHeader(&buf, salient, false, false))
	assert.Contains(t, buf.String(), "  [long query: ColBERT and FTS on its 64 most salient terms]\n")
	assert.NotContains(t, buf.String(), "ranked on")
}

func TestWriteAskHeader_OmitsCapLineWhenNotCapped(t *testing.T) {
	result := cappedHeaderResult()
	result.QueryCapped = nil
	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, result, false, true))
	assert.NotContains(t, buf.String(), "long query")
	assert.True(t, strings.HasPrefix(buf.String(), "Search:"))
}
