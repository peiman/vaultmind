package query_test

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The keyword-only hint reaches every kind of vault, most of them knowledge
// bases. Its example was an agent's arc ("The Judgment Gap"), which reads as
// noise in a project's docs vault.
func TestKeywordOnlyHint_ItsExampleFitsAKnowledgeVault(t *testing.T) {
	var buf bytes.Buffer
	require.True(t, query.WriteKeywordOnlyHint(&buf, "keyword", 0))
	out := buf.String()
	assert.NotContains(t, out, "an arc")
	assert.NotContains(t, out, "Judgment Gap")
	assert.Contains(t, out, "index --embed", "the remedy stays")
}
