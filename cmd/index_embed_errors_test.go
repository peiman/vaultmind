package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// STRANGER TEST (2026-09-23): 28 of 60 notes failed to embed and `index --json`
// said {"status":"ok","warnings":[],"errors":[]}. The count sat in
// result.embed.errors where nothing that checks status would look. Those notes
// are invisible to semantic search — that is a warning, and it must be one.

func TestIndexEnvelope_EmbedFailuresAreAWarning(t *testing.T) {
	env := indexEnvelope(index.IndexAndEmbedResult{
		Index: &index.IndexResult{},
		Embed: &index.EmbedResult{Embedded: 32, Errors: 28},
	}, "./v")
	assert.Equal(t, "warning", env.Status)
	require.Len(t, env.Warnings, 1)
	assert.Equal(t, warnCodeEmbedErrors, env.Warnings[0].Code)
	assert.Contains(t, env.Warnings[0].Message, "28")
	assert.Contains(t, env.Warnings[0].Message, "semantic search")
}

func TestIndexEnvelope_CleanRunStaysOK(t *testing.T) {
	env := indexEnvelope(index.IndexAndEmbedResult{
		Index: &index.IndexResult{},
		Embed: &index.EmbedResult{Embedded: 60},
	}, "./v")
	assert.Equal(t, "ok", env.Status)
	assert.Empty(t, env.Warnings)
}

func TestFormatIndexResult_SaysWhatEmbedErrorsMean(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, formatIndexResult(index.IndexAndEmbedResult{
		Index: &index.IndexResult{},
		Embed: &index.EmbedResult{Embedded: 32, Errors: 28},
	}, "bge-m3", &buf))
	out := buf.String()
	assert.True(t, strings.Contains(out, "⚠ 28 note(s) have no embedding"), out)
}
