package mutation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDetectLineEnding_LF(t *testing.T) {
	assert.Equal(t, "\n", DetectLineEnding([]byte("---\nid: test\n")))
}

func TestDetectLineEnding_CRLF(t *testing.T) {
	assert.Equal(t, "\r\n", DetectLineEnding([]byte("---\r\nid: test\r\n")))
}

func TestDetectLineEnding_Empty(t *testing.T) {
	assert.Equal(t, "\n", DetectLineEnding([]byte{}))
}

func TestDetectLineEnding_NoNewlines(t *testing.T) {
	assert.Equal(t, "\n", DetectLineEnding([]byte("no newlines")))
}

func TestDetectTrailingNewline_Yes(t *testing.T) {
	assert.True(t, DetectTrailingNewline([]byte("content\n")))
}

func TestDetectTrailingNewline_No(t *testing.T) {
	assert.False(t, DetectTrailingNewline([]byte("content")))
}

func TestDetectTrailingNewline_CRLF(t *testing.T) {
	assert.True(t, DetectTrailingNewline([]byte("content\r\n")))
}

func TestDetectTrailingNewline_Empty(t *testing.T) {
	assert.False(t, DetectTrailingNewline([]byte{}))
}

func TestParseFrontmatterNode_Basic(t *testing.T) {
	raw := []byte("---\nid: test-note\ntype: project\nstatus: active\n---\n# Hello World\n")
	node, bodyOffset, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Greater(t, bodyOffset, 0)
	assert.Equal(t, int(yaml.DocumentNode), int(node.Kind))
	assert.Equal(t, int(yaml.MappingNode), int(node.Content[0].Kind))
	body := string(raw[bodyOffset:])
	assert.Equal(t, "# Hello World\n", body)
}

func TestParseFrontmatterNode_NoFrontmatter(t *testing.T) {
	raw := []byte("# Just a heading\nSome content.\n")
	_, _, err := ParseFrontmatterNode(raw)
	assert.Error(t, err)
}

func TestParseFrontmatterNode_EmptyFrontmatter(t *testing.T) {
	raw := []byte("---\n---\n# Body\n")
	node, bodyOffset, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	assert.NotNil(t, node)
	body := string(raw[bodyOffset:])
	assert.Equal(t, "# Body\n", body)
}

func TestSerializeFrontmatter_Basic(t *testing.T) {
	raw := []byte("---\nid: test-note\ntype: project\n---\n# Body\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)

	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte("---\n")))
	assert.True(t, bytes.HasSuffix(out, []byte("---\n")))
	assert.Contains(t, string(out), "id: test-note")
	assert.Contains(t, string(out), "type: project")
}

func TestSerializeFrontmatter_CRLF(t *testing.T) {
	raw := []byte("---\nid: test\ntype: project\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)

	out, err := SerializeFrontmatter(node, "\r\n")
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte("---\r\n")))
	assert.Contains(t, string(out), "\r\n")
}

func TestRoundTrip_PreservesKeyOrder(t *testing.T) {
	raw := []byte("---\nstatus: active\nid: test\ntype: project\ntitle: My Note\n---\n# Body\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)

	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)

	statusIdx := bytes.Index(out, []byte("status:"))
	idIdx := bytes.Index(out, []byte("id:"))
	assert.Less(t, statusIdx, idIdx, "key order should be preserved")
}

func TestSetKey_ExistingKey(t *testing.T) {
	raw := []byte("---\nid: test\nstatus: active\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	err = SetKey(node.Content[0], "status", "paused")
	require.NoError(t, err)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "status: paused")
	assert.NotContains(t, string(out), "status: active")
}

func TestSetKey_NewKey(t *testing.T) {
	raw := []byte("---\nid: test\ntype: project\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	err = SetKey(node.Content[0], "status", "active")
	require.NoError(t, err)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "status: active")
}

func TestSetKey_ListValue(t *testing.T) {
	raw := []byte("---\nid: test\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	err = SetKey(node.Content[0], "tags", []interface{}{"billing", "payments"})
	require.NoError(t, err)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "tags:")
	assert.Contains(t, string(out), "- billing")
	assert.Contains(t, string(out), "- payments")
}

func TestSetKey_PreservesOtherKeys(t *testing.T) {
	raw := []byte("---\nid: test\ntype: project\nstatus: active\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	err = SetKey(node.Content[0], "status", "paused")
	require.NoError(t, err)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.Contains(t, string(out), "id: test")
	assert.Contains(t, string(out), "type: project")
}

func TestUnsetKey_ExistingKey(t *testing.T) {
	raw := []byte("---\nid: test\nstatus: active\ntags:\n  - billing\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	removed := UnsetKey(node.Content[0], "tags")
	assert.True(t, removed)
	out, err := SerializeFrontmatter(node, "\n")
	require.NoError(t, err)
	assert.NotContains(t, string(out), "tags")
	assert.Contains(t, string(out), "id: test")
	assert.Contains(t, string(out), "status: active")
}

func TestUnsetKey_NonExistentKey(t *testing.T) {
	raw := []byte("---\nid: test\n---\n")
	node, _, err := ParseFrontmatterNode(raw)
	require.NoError(t, err)
	removed := UnsetKey(node.Content[0], "nonexistent")
	assert.False(t, removed)
}

func TestSpliceFile_Basic(t *testing.T) {
	original := []byte("---\nid: test\nstatus: active\n---\n# Hello World\n\nSome content.\n")
	newFM := []byte("---\nid: test\nstatus: paused\n---\n")
	_, bodyOffset, err := ParseFrontmatterNode(original)
	require.NoError(t, err)
	result := SpliceFile(original, newFM, bodyOffset)
	assert.True(t, bytes.HasPrefix(result, []byte("---\nid: test\nstatus: paused\n---\n")))
	assert.Contains(t, string(result), "# Hello World")
	assert.Contains(t, string(result), "Some content.")
}

func TestSpliceFile_PreservesBodyBytesExactly(t *testing.T) {
	body := "# Title\n\n  indented line  \n\ttab line\ntrailing spaces   \n"
	original := []byte("---\nid: test\n---\n" + body)
	newFM := []byte("---\nid: test\nstatus: active\n---\n")
	_, bodyOffset, err := ParseFrontmatterNode(original)
	require.NoError(t, err)
	result := SpliceFile(original, newFM, bodyOffset)
	resultBody := string(result[len(newFM):])
	assert.Equal(t, body, resultBody)
}

func TestSpliceFile_NormalizesGap(t *testing.T) {
	original := []byte("---\nid: test\n---\n\n\n\n# Body\n")
	newFM := []byte("---\nid: test\nstatus: active\n---\n")
	_, bodyOffset, err := ParseFrontmatterNode(original)
	require.NoError(t, err)
	result := SpliceFile(original, newFM, bodyOffset)
	expected := "---\nid: test\nstatus: active\n---\n\n# Body\n"
	assert.Equal(t, expected, string(result))
}

func TestSpliceFile_PreservesTrailingNewline(t *testing.T) {
	original := []byte("---\nid: test\n---\n# Body\n")
	newFM := []byte("---\nid: test\n---\n")
	_, bodyOffset, err := ParseFrontmatterNode(original)
	require.NoError(t, err)
	result := SpliceFile(original, newFM, bodyOffset)
	assert.True(t, DetectTrailingNewline(result))
}

func TestSpliceFile_PreservesNoTrailingNewline(t *testing.T) {
	original := []byte("---\nid: test\n---\n# Body")
	newFM := []byte("---\nid: test\n---\n")
	_, bodyOffset, err := ParseFrontmatterNode(original)
	require.NoError(t, err)
	result := SpliceFile(original, newFM, bodyOffset)
	assert.False(t, DetectTrailingNewline(result))
}

func TestGenerateDiff_ShowsChanges(t *testing.T) {
	original := "---\nid: test\nstatus: active\n---\n# Body\n"
	modified := "---\nid: test\nstatus: paused\n---\n# Body\n"
	diff := GenerateDiff("notes/test.md", original, modified)
	assert.Contains(t, diff, "-status: active")
	assert.Contains(t, diff, "+status: paused")
	assert.Contains(t, diff, "--- a/notes/test.md")
	assert.Contains(t, diff, "+++ b/notes/test.md")
}

func TestGenerateDiff_NoDifference(t *testing.T) {
	content := "---\nid: test\n---\n# Body\n"
	diff := GenerateDiff("notes/test.md", content, content)
	assert.Empty(t, diff)
}

func TestSetKey_NonMappingNode(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "scalar"}
	err := SetKey(node, "key", "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected mapping node")
}

func TestUnsetKey_NonMappingNode(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: "scalar"}
	removed := UnsetKey(node, "key")
	assert.False(t, removed)
}

func TestParseFrontmatterNode_NoClosingDelimiter(t *testing.T) {
	raw := []byte("---\nid: test\ntitle: No closing\n")
	_, _, err := ParseFrontmatterNode(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closing --- not found")
}

func TestParseFrontmatterNode_NoNewlineAfterOpening(t *testing.T) {
	raw := []byte("---")
	_, _, err := ParseFrontmatterNode(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no newline")
}

func TestSpliceFile_NoBody(t *testing.T) {
	original := []byte("---\nid: test\n---\n")
	newFM := []byte("---\nid: test\nstatus: active\n---\n")
	_, bodyOffset, err := ParseFrontmatterNode(original)
	require.NoError(t, err)
	result := SpliceFile(original, newFM, bodyOffset)
	assert.True(t, bytes.HasPrefix(result, []byte("---\nid: test\nstatus: active\n---\n")))
}
