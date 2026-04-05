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
