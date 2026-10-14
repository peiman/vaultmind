package query_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
)

func TestWriteKeywordOnlyHint_FiresOnKeywordModeWithZeroHits(t *testing.T) {
	var buf bytes.Buffer
	wrote := query.WriteKeywordOnlyHint(&buf, "keyword", 0)
	assert.True(t, wrote, "hint should be written when keyword mode returns zero hits")
	out := buf.String()
	assert.Contains(t, out, "no embeddings", "hint should name the cause")
	assert.Contains(t, out, "vaultmind index --embed", "hint should name the remedy")
	assert.NotContains(t, out, "--model bge-m3",
		"the no-embeddings remedy must run on any backend — bge-m3 is refused on the pure-Go binary")
}

func TestWriteKeywordOnlyHint_SilentOnHybridMode(t *testing.T) {
	var buf bytes.Buffer
	wrote := query.WriteKeywordOnlyHint(&buf, "hybrid", 0)
	assert.False(t, wrote, "hybrid mode with zero hits is a different problem; no hint")
	assert.Empty(t, buf.String())
}

func TestWriteKeywordOnlyHint_SilentWhenKeywordReturnsHits(t *testing.T) {
	var buf bytes.Buffer
	wrote := query.WriteKeywordOnlyHint(&buf, "keyword", 3)
	assert.False(t, wrote, "if keyword search found something the user got what they asked for")
	assert.Empty(t, buf.String())
}

func TestWriteSparseFormatStaleNotice_HookCallerIsSilent(t *testing.T) {
	for _, caller := range []string{"vaultmind-userprompt-hook", "vaultmind-persona-hook", "vaultmind-reach-hook", "auto-rag-guard"} {
		t.Run(caller, func(t *testing.T) {
			t.Setenv("VAULTMIND_CALLER", caller)
			var buf bytes.Buffer
			query.WriteSparseFormatStaleNotice(&buf)
			assert.Empty(t, buf.String())
		})
	}
}

func TestWriteSparseFormatStaleNotice_PlainAskPrints(t *testing.T) {
	for _, caller := range []string{"", "cli"} {
		t.Run(caller, func(t *testing.T) {
			t.Setenv("VAULTMIND_CALLER", caller)
			var buf bytes.Buffer
			query.WriteSparseFormatStaleNotice(&buf)
			assert.Contains(t, buf.String(), index.SparseFormatStaleNotice)
			assert.Contains(t, buf.String(), index.SparseFormatStaleRemedy)
		})
	}
}

func TestWriteKeywordOnlyHint_EndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	_ = query.WriteKeywordOnlyHint(&buf, "keyword", 0)
	assert.True(t, strings.HasSuffix(buf.String(), "\n"), "hint should terminate cleanly for CLI output")
}
