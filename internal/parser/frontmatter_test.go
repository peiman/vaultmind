package parser_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFrontmatter_WithFrontmatter(t *testing.T) {
	input := []byte("---\nid: proj-payment-retries\ntype: project\nstatus: active\ntitle: Payment Retries\naliases:\n  - Retry Engine\n  - Billing Retries\ncreated: 2026-04-03\nvm_updated: 2026-04-03\ntags:\n  - billing\n  - payments\nrelated_ids:\n  - concept-idempotency\n---\n\n# Body starts here\n\nSome body text.")

	fm, body, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)

	assert.Equal(t, "proj-payment-retries", fm["id"])
	assert.Equal(t, "project", fm["type"])
	assert.Equal(t, "active", fm["status"])
	assert.Equal(t, "Payment Retries", fm["title"])
	assert.Contains(t, body, "# Body starts here")
	assert.NotContains(t, body, "---")
}

func TestExtractFrontmatter_NoFrontmatter(t *testing.T) {
	input := []byte("# Just a heading\n\nSome plain text with no frontmatter.")

	fm, body, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)

	assert.Empty(t, fm)
	assert.Contains(t, body, "# Just a heading")
}

func TestExtractFrontmatter_EmptyFrontmatter(t *testing.T) {
	input := []byte("---\n---\n\nBody text here.")

	fm, body, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)

	assert.Empty(t, fm)
	assert.Contains(t, body, "Body text here.")
}

func TestExtractFrontmatter_InvalidYAML(t *testing.T) {
	input := []byte("---\nid: [unclosed bracket\ntype: project\n---\n\nBody text.")

	_, _, err := parser.ExtractFrontmatter(input)
	assert.Error(t, err)
}

func TestExtractFrontmatter_ScalarAliasesPreserved(t *testing.T) {
	input := []byte("---\nid: note-x\ntype: concept\naliases: scalar-alias\n---\n")

	fm, _, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)
	assert.Equal(t, "scalar-alias", fm["aliases"])
}

func TestExtractFrontmatter_YAMLListValues(t *testing.T) {
	input := []byte("---\nid: note-y\ntype: concept\naliases:\n  - First Alias\n  - Second Alias\ntags:\n  - alpha\n  - beta\nrelated_ids:\n  - concept-foo\n  - concept-bar\n---\n")

	fm, _, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)

	aliases, ok := fm["aliases"].([]interface{})
	require.True(t, ok, "aliases should be a []interface{}")
	assert.Len(t, aliases, 2)
	assert.Equal(t, "First Alias", aliases[0])

	tags, ok := fm["tags"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, tags, "alpha")

	relatedIDs, ok := fm["related_ids"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, relatedIDs, "concept-foo")
}

func TestExtractFrontmatter_OnlyClosingDelimiter(t *testing.T) {
	input := []byte("# Heading\n\nSome content.\n---\nMore content after a horizontal rule.")

	fm, body, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)
	assert.Empty(t, fm)
	assert.Contains(t, body, "# Heading")
}

func TestClassifyNote_DomainNote(t *testing.T) {
	fm := map[string]interface{}{"id": "proj-payment-retries", "type": "project"}
	isDomain, id, noteType := parser.ClassifyNote(fm)

	assert.True(t, isDomain)
	assert.Equal(t, "proj-payment-retries", id)
	assert.Equal(t, "project", noteType)
}

func TestClassifyNote_MissingID(t *testing.T) {
	fm := map[string]interface{}{"type": "project"}
	isDomain, id, noteType := parser.ClassifyNote(fm)

	assert.False(t, isDomain)
	assert.Empty(t, id)
	assert.Empty(t, noteType)
}

func TestClassifyNote_MissingType(t *testing.T) {
	fm := map[string]interface{}{"id": "proj-payment-retries"}
	isDomain, id, noteType := parser.ClassifyNote(fm)

	assert.False(t, isDomain)
	assert.Empty(t, id)
	assert.Empty(t, noteType)
}

func TestClassifyNote_EmptyFrontmatter(t *testing.T) {
	isDomain, id, noteType := parser.ClassifyNote(nil)

	assert.False(t, isDomain)
	assert.Empty(t, id)
	assert.Empty(t, noteType)
}

func TestClassifyNote_NonStringID(t *testing.T) {
	fm := map[string]interface{}{"id": 42, "type": "project"}
	isDomain, id, noteType := parser.ClassifyNote(fm)

	assert.False(t, isDomain)
	assert.Empty(t, id)
	assert.Empty(t, noteType)
}

func TestClassifyNote_EmptyStringID(t *testing.T) {
	fm := map[string]interface{}{"id": "", "type": "project"}
	isDomain, id, noteType := parser.ClassifyNote(fm)

	assert.False(t, isDomain)
	assert.Empty(t, id)
	assert.Empty(t, noteType)
}

func TestExtractFrontmatter_RealVaultNote(t *testing.T) {
	input := []byte("---\nid: concept-act-r\ntype: concept\ntitle: ACT-R\ncreated: 2026-04-03\nvm_updated: 2026-04-03\naliases:\n  - Adaptive Control of Thought-Rational\n  - ACT-R Architecture\ntags:\n  - cognitive-science\n  - cognitive-architecture\nrelated_ids:\n  - concept-spreading-activation\n  - concept-forgetting-curve\nsource_ids:\n  - source-anderson-1983\n---\n\n## Overview\n\nACT-R is a cognitive architecture.\n\n## Connections\n\nSee [[Context Pack]] and [[Spreading Activation]].")

	fm, body, err := parser.ExtractFrontmatter(input)
	require.NoError(t, err)

	isDomain, id, noteType := parser.ClassifyNote(fm)
	assert.True(t, isDomain)
	assert.Equal(t, "concept-act-r", id)
	assert.Equal(t, "concept", noteType)
	assert.Contains(t, body, "## Overview")
	assert.Contains(t, body, "[[Context Pack]]")
}
