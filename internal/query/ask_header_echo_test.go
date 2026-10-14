package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAskHeader_ShortQueryEchoIsByteIdentical(t *testing.T) {
	require.Equal(t, 160, askHeaderQueryRunes, "one bound for the echoed query")
	query := "what does the index keep\n\"quoted\""
	result := &AskResult{
		Query:   query,
		TopHits: []retrieval.ScoredResult{{ID: "n", Title: "N"}},
	}
	var got bytes.Buffer
	require.NoError(t, writeAskHeader(&got, result, false, true))
	want := fmt.Sprintf("Search: %q (%d hits)\n", query, 1)
	assert.Equal(t, want, got.String(), "a query within the bound is the old header, byte for byte")
	assert.Equal(t, query, result.Query)
}

func TestWriteAskHeader_LongQueryEchoIsTruncated(t *testing.T) {
	tail := "UNIQUE_TAIL_TOKEN_ZEBRA"
	query := strings.Repeat("あ", askHeaderQueryRunes) + tail
	result := &AskResult{
		Query:   query,
		TopHits: []retrieval.ScoredResult{{ID: "how-it-works", Title: "How it works"}},
		Context: &memory.ContextPackResult{TargetID: "how-it-works"},
	}
	var got bytes.Buffer
	require.NoError(t, formatAskWithOptions(result, &got, formatOpts{}))
	out := got.String()

	chars := len([]rune(query))
	head := string([]rune(query)[:askHeaderQueryRunes])
	wantHeader := fmt.Sprintf("Search: %q… (%d chars) (%d hits)\n", head, chars, 1)
	assert.True(t, strings.HasPrefix(out, wantHeader), "header:\n%s", out)
	assert.Contains(t, out, "Context from: how-it-works")
	assert.NotContains(t, out, tail, "the recalled text must not carry the rest of a pasted prompt")
	assert.NotContains(t, out, strings.Repeat("あ", askHeaderQueryRunes+1))

	raw, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(raw), tail, "result.query keeps the full text")
	assert.Equal(t, query, result.Query)
}
