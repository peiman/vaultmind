package parser_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findClosingDelimiter has branches for "--- at end of string", "--- followed
// by \r\n", and "--- followed by something else (not standalone line)".
// These tests exercise each so a refactor can't silently drop one.

// CRLF line endings: frontmatter must still parse when the file came from
// Windows. Regression: a CRLF-only branch that the unit tests missed would
// break every vault synced from a Windows editor.
func TestExtractFrontmatter_CRLFLineEndings(t *testing.T) {
	content := []byte("---\r\nid: c-1\r\ntype: concept\r\n---\r\nbody here\r\n")
	fm, body, err := parser.ExtractFrontmatter(content)
	require.NoError(t, err)
	require.NotNil(t, fm)
	assert.Equal(t, "c-1", fm["id"])
	assert.Contains(t, body, "body here")
}

// Frontmatter ending at EOF (no trailing newline after closing "---") must
// still parse. Handwritten files sometimes arrive without the trailing
// newline, and if we reject them users see confusing "no frontmatter"
// behavior.
func TestExtractFrontmatter_ClosingDelimiterAtEOF(t *testing.T) {
	content := []byte("---\nid: c-1\ntype: concept\n---")
	fm, body, err := parser.ExtractFrontmatter(content)
	require.NoError(t, err)
	require.NotNil(t, fm, "frontmatter at EOF must still parse")
	assert.Equal(t, "c-1", fm["id"])
	assert.Empty(t, body, "no body after closing delimiter")
}

// A string that looks like it might have a closing "---" but that's part of
// the YAML content (not on its own line) must NOT terminate the frontmatter
// block. The closing delimiter contract is "standalone line".
func TestExtractFrontmatter_InlineDashesDontTerminate(t *testing.T) {
	// YAML string value "foo --- bar" is legal; --- within a quoted string
	// isn't a delimiter.
	content := []byte("---\nid: c-1\ntype: concept\ntitle: \"foo --- bar\"\n---\nbody\n")
	fm, body, err := parser.ExtractFrontmatter(content)
	require.NoError(t, err)
	require.NotNil(t, fm)
	assert.Equal(t, "foo --- bar", fm["title"])
	assert.Contains(t, body, "body")
}

// No closing delimiter at all: the function documents that it returns nil
// frontmatter and the full content as body. Losing this contract would
// cause the indexer to treat half-written notes as empty.
func TestExtractFrontmatter_NoClosingDelimiterReturnsOriginal(t *testing.T) {
	content := []byte("---\nid: c-1\nbut no closing delimiter\nhere\n")
	fm, body, err := parser.ExtractFrontmatter(content)
	require.NoError(t, err)
	assert.Nil(t, fm)
	assert.Equal(t, string(content), body)
}
