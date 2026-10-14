package query

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

// TestShapeQuery_LowDensityHeadStillAppliesForwardBound: a head of whitespace
// or separators that sits between the forward window (1152×8 bytes) and the
// model-cap window (BGEM3MaxTokens×8) must still be cut at the forward window.
// The head is deliberately shorter than the model cap and longer than the
// forward window. A 100K head is outside both, so both paths cut at the
// forward window and sizing the read from maxTokens stays green.
func TestShapeQuery_LowDensityHeadStillAppliesForwardBound(t *testing.T) {
	const (
		forward  = 1152 // app.ask.query_forward_tokens default
		sentinel = "zzboundmarker"
		spaces   = 30_000
		dots     = 12_000
	)
	emb := requireSpanEmbedder(t)
	prose := proseWords(5000, sentinel)
	tests := []struct {
		name string
		head string
	}{
		{name: "whitespace", head: strings.Repeat(" ", spaces)},
		{name: "separators", head: strings.Repeat(".", dots)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.head + prose
			window := embedding.QuerySpanWindow(text, forward)
			modelWindow := embedding.QuerySpanWindow(text, embedding.BGEM3MaxTokens)
			require.Less(t, len(window), len(tc.head), "forward window must end inside the head")
			require.Greater(t, len(modelWindow), len(tc.head), "model-cap window must reach the prose, or the maxTokens mutation stays green")

			buf := captureDebugLog(t)
			got := ShapeQuery(context.Background(), text, forward, 0, emb)
			require.NotNil(t, got.Report)
			require.NotContains(t, got.EmbedText, sentinel)
			require.Equal(t, window, got.EmbedText)
			require.Contains(t, buf.String(), "ask query shape: span window under-counted; cutting at the window")
		})
	}
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShapeQuery(context.Background(), tc.text, 1152, 0, emb)
			require.Equal(t, tc.text, got.EmbedText)
			require.Nil(t, got.Report)
		})
	}
}
