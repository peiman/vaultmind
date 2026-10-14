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
		QueryCapped:       &QueryCapped{UsedWords: 64, TotalWords: 912},
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
	assert.Equal(t, "  [query capped: ranked on the first 64 of 912 words]", lines[1])
	assert.Contains(t, lines[2], "relevance math:")
	assert.Contains(t, buf.String(), "[federated:")
}

func TestWriteAskHeader_OmitsCapLineWhenNotCapped(t *testing.T) {
	result := cappedHeaderResult()
	result.QueryCapped = nil
	var buf bytes.Buffer
	require.NoError(t, writeAskHeader(&buf, result, false, true))
	assert.NotContains(t, buf.String(), "query capped")
	assert.True(t, strings.HasPrefix(buf.String(), "Search:"))
}
