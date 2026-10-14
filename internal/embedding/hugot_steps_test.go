package embedding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/gomlx/go-huggingface/tokenizers/hftokenizer"
	"github.com/knights-analytics/hugot/backends"
	"github.com/stretchr/testify/require"
)

func TestEnableContentTokenSpans_CJKRunIsManyTokens(t *testing.T) {
	path := filepath.Join(DefaultCacheDir(), "BAAI_bge-m3", "tokenizer.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("bge-m3 tokenizer.json is not in the model cache")
	}
	tk, err := hftokenizer.NewFromContent(nil, raw)
	require.NoError(t, err)
	// The loader's option set: specials on, spans off.
	require.NoError(t, tk.With(api.EncodeOptions{AddSpecialTokens: true, MaxLen: 8192}))
	wrapped := &backends.Tokenizer{
		GoTokenizer:      &backends.GoTokenizer{Tokenizer: tk},
		MaxAllowedTokens: 8192,
	}

	text := strings.Repeat("日本語の文章", 40)
	require.Equal(t, 1, len(strings.Fields(text)))
	bare := tk.EncodeWithAnnotations(text)
	require.Empty(t, bare.Spans)

	e := &BGEM3Embedder{preprocess: func(batch *backends.PipelineBatch, texts []string) error {
		tokenizeInputs(batch, wrapped, texts)
		return nil
	}}
	spans, err := e.TokenSpans(text)
	require.NoError(t, err)
	require.Greater(t, len(spans), len(strings.Fields(text)))
	for _, sp := range spans {
		require.Greater(t, sp[1], sp[0])
		require.LessOrEqual(t, sp[1], len(text))
		require.True(t, strings.HasPrefix(text, text[:sp[1]]))
	}
}
