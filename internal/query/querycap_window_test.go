package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireSpanEmbedder loads the real BGE-M3 tokenizer and nothing else.
// The model weights stay on disk. CI=true fails when the file is missing;
// a local run skips and names the fetch.
func requireSpanEmbedder(t *testing.T) *embedding.BGEM3Embedder {
	t.Helper()
	raw, err := embedding.ReadBGEM3Tokenizer()
	if err != nil {
		msg := embedding.TokenizerFetchHint() + ": " + err.Error()
		if embedding.TokenizerMissIsFatal(os.Getenv("CI")) {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	emb, err := embedding.NewSpanEmbedder(raw, embedding.BGEM3MaxTokens)
	require.NoError(t, err)
	return emb
}

func captureDebugLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	old := log.Logger
	buf := &bytes.Buffer{}
	log.Logger = zerolog.New(buf).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = old })
	return buf
}

func proseWords(n int, first string) string {
	words := make([]string, n)
	words[0] = first
	for i := 1; i < n; i++ {
		words[i] = fmt.Sprintf("word%d", i)
	}
	return strings.Join(words, " ")
}

// lowDensityHead is a long low-token head that collapse does not shorten.
// Each same-character run is exactly RepeatedNonAlnumCap, broken by one
// other character, so the head stays between the forward window and the
// model-cap window. A pure run of one character would collapse to 16 and
// the window check would no longer be about sizing.
func lowDensityHead(unit string) string {
	return strings.Repeat(unit, 1800)
}

// TestShapeQuery_LowDensityHeadStillAppliesForwardBound: a head of whitespace
// or separators that sits between the forward window (1152×8 bytes) and the
// model-cap window (BGEM3MaxTokens×8) must still be cut at the forward window.
// The head is deliberately shorter than the model cap and longer than the
// forward window. A head outside both makes both paths cut at the forward
// window, and sizing the read from maxTokens stays green.
func TestShapeQuery_LowDensityHeadStillAppliesForwardBound(t *testing.T) {
	const (
		forward  = 1152 // app.ask.query_forward_tokens default
		sentinel = "zzboundmarker"
	)
	emb := requireSpanEmbedder(t)
	prose := proseWords(5000, sentinel)
	cap := strings.Repeat(" ", embedding.RepeatedNonAlnumCap)
	dot := strings.Repeat(".", embedding.RepeatedNonAlnumCap)
	tests := []struct {
		name string
		head string
	}{
		{name: "whitespace", head: lowDensityHead(cap + ".")},
		{name: "separators", head: lowDensityHead(dot + " ")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.head + prose
			require.Equal(t, tc.head, embedding.CapSymbolRuns(tc.head))
			window := embedding.QuerySpanWindow(text, forward)
			modelWindow := embedding.QuerySpanWindow(text, embedding.BGEM3MaxTokens)
			require.Less(t, len(window), len(tc.head), "forward window must end inside the head")
			require.Greater(t, len(modelWindow), len(tc.head), "model-cap window must reach the prose, or the maxTokens mutation stays green")

			buf := captureDebugLog(t)
			got := ShapeQuery(context.Background(), text, forward, 0, emb)
			require.NotNil(t, got.Report)
			require.NotContains(t, got.EmbedText, sentinel)
			require.Equal(t, window, got.EmbedText)
			require.Zero(t, got.Report.TotalTokens)
			require.Equal(t, utf8.RuneCountInString(text), got.Report.TotalChars)
			raw, err := json.Marshal(got.Report)
			require.NoError(t, err)
			require.NotContains(t, string(raw), "total_tokens")
			require.Contains(t, string(raw), `"total_chars":`)
			require.Contains(t, buf.String(), "ask query shape: span window under-counted; cutting at the window")
		})
	}
}

// TestShapeQuery_MixedSymbolRunStaysFast: 12,000 unspaced mixed punctuation
// characters plus prose must shape in under two seconds with the real
// tokenizer. A cap that only shortens a run of one character leaves this
// text untouched, and this test goes red.
func TestShapeQuery_MixedSymbolRunStaysFast(t *testing.T) {
	emb := requireSpanEmbedder(t)
	prose := " alpha beta gamma"
	symbols := strings.Repeat(".-", 6_000)
	require.Equal(t, 12_000, utf8.RuneCountInString(symbols))
	text := symbols + prose
	// Stop at 2s. A same-character-only cap leaves this run intact, and
	// tokenizing it runs for minutes; waiting that out would hide the miss.
	type shaped struct {
		got     ShapedQuery
		elapsed time.Duration
	}
	ch := make(chan shaped, 1)
	go func() {
		start := time.Now()
		got := ShapeQuery(context.Background(), text, 1152, 0, emb)
		ch <- shaped{got, time.Since(start)}
	}()
	select {
	case res := <-ch:
		t.Logf("shaping 12000 mixed symbols took %s", res.elapsed)
		require.Lessf(t, res.elapsed, 2*time.Second, "shaping 12000 mixed symbols took %s", res.elapsed)
		require.Equal(t, strings.Repeat(".-", embedding.RepeatedNonAlnumCap/2)+prose, res.got.EmbedText)
		require.NotContains(t, res.got.EmbedText, strings.Repeat(".-", 9))
	case <-time.After(2 * time.Second):
		t.Fatal("shaping 12000 mixed symbols exceeded 2s")
	}
}

// TestShapeQuery_RepeatedCharacterRunStaysFast: 12,000 dots plus prose must
// shape in under two seconds with the real tokenizer. Removing the cap
// tokenizes a multi-kilobyte run of one character and this test goes red.
func TestShapeQuery_RepeatedCharacterRunStaysFast(t *testing.T) {
	emb := requireSpanEmbedder(t)
	prose := " alpha beta gamma"
	text := strings.Repeat(".", 12_000) + prose
	start := time.Now()
	got := ShapeQuery(context.Background(), text, 1152, 0, emb)
	elapsed := time.Since(start)
	require.Lessf(t, elapsed, 2*time.Second, "shaping 12000 dots took %s", elapsed)
	require.Contains(t, got.EmbedText, "alpha beta gamma")
	require.NotContains(t, got.EmbedText, strings.Repeat(".", 17))
}

// TestShapeQuery_NormalProseMatchesFullTokenization: ordinary prose is cut on
// a real token boundary from a full tokenize, not at the character window.
func TestShapeQuery_NormalProseMatchesFullTokenization(t *testing.T) {
	const forward = 1152
	emb := requireSpanEmbedder(t)
	prose := proseWords(5000, "word0")
	full, err := emb.TokenSpans(prose, 0)
	require.NoError(t, err)
	want, _, _, applied := ForwardQuery(prose, forward, full)
	require.True(t, applied)

	buf := captureDebugLog(t)
	got := ShapeQuery(context.Background(), prose, forward, 0, emb)
	require.Equal(t, want, got.EmbedText)
	require.NotEqual(t, embedding.QuerySpanWindow(prose, forward), got.EmbedText)
	require.NotContains(t, buf.String(), "span window under-counted")
	require.NotNil(t, got.Report)
	require.Greater(t, got.Report.TotalTokens, forward)
	require.Zero(t, got.Report.TotalChars)
}

func TestShapeQuery_ShortQueryIsByteIdentical(t *testing.T) {
	emb := requireSpanEmbedder(t)
	tests := []struct {
		name string
		text string
	}{
		{name: "empty", text: ""},
		{name: "one word", text: "vaultmind"},
		{name: "sentence", text: "where did we park the tokenizer pin"},
		{name: "run of 16", text: "end" + strings.Repeat(".", 16) + " next"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShapeQuery(context.Background(), tc.text, 1152, 0, emb)
			require.Equal(t, tc.text, got.EmbedText)
			require.Nil(t, got.Report)
		})
	}
}

// oneSpan is a tokenizer that reports a single content span, so a long
// string takes the under-count path without the real tokenizer.
type oneSpan struct{}

func (oneSpan) AnalyzeQuery(context.Context, string) (embedding.QueryTokens, error) {
	return embedding.QueryTokens{}, nil
}

func (oneSpan) TokenSpans(string, int) ([][2]int, error) {
	return [][2]int{{0, 1}}, nil
}

func TestShapeQuery_UncountedTotalReportsOriginalCharacterCount(t *testing.T) {
	original := strings.Repeat("…", 20) + strings.Repeat(".-", 30)
	got := ShapeQuery(context.Background(), original, 2, 0, oneSpan{})
	require.NotNil(t, got.Report)
	assert.Equal(t, 2, got.Report.EmbeddedTokens)
	assert.Zero(t, got.Report.TotalTokens)
	assert.Equal(t, utf8.RuneCountInString(original), got.Report.TotalChars)
	assert.Equal(t, 80, got.Report.TotalChars)
	raw, err := json.Marshal(got.Report)
	require.NoError(t, err)
	assert.JSONEq(t, `{"embedded_tokens":2,"total_chars":80,"salient_terms":0}`, string(raw))
}

func TestAskHits_EchoesTheQueryWhileRankingTextCollapses(t *testing.T) {
	original := strings.Repeat(".", 100) + " keep"
	lane := &ftsTextRetriever{}
	res, err := AskHits(context.Background(), lane, original, 5, 1152, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, original, res.Query)
	assert.Equal(t, strings.Repeat(".", 16)+" keep", res.rankingQuery)
}
