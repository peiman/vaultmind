package query_test

import (
	"bytes"
	"strings"
	"testing"

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

func TestWriteKeywordOnlyHint_EndsWithNewline(t *testing.T) {
	var buf bytes.Buffer
	_ = query.WriteKeywordOnlyHint(&buf, "keyword", 0)
	assert.True(t, strings.HasSuffix(buf.String(), "\n"), "hint should terminate cleanly for CLI output")
}
