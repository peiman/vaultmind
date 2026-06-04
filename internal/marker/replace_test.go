package marker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceRegion_Basic(t *testing.T) {
	raw := []byte("# Title\n\n<!-- VAULTMIND:GENERATED:related:START -->\nold content\n<!-- VAULTMIND:GENERATED:related:END -->\n\nMore text.\n")
	newContent := []byte("new content\n")
	result, err := ReplaceRegion(raw, "related", newContent, false)
	require.NoError(t, err)
	s := string(result)
	assert.Contains(t, s, "new content")
	assert.NotContains(t, s, "old content")
	assert.Contains(t, s, "<!-- VAULTMIND:GENERATED:related:START -->")
	assert.Contains(t, s, "<!-- VAULTMIND:GENERATED:related:END -->")
	assert.Contains(t, s, "<!-- checksum:")
	assert.Contains(t, s, "# Title")
	assert.Contains(t, s, "More text.")
}

func TestReplaceRegion_InsertsChecksum(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\nold\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	newContent := []byte("new content\n")
	result, err := ReplaceRegion(raw, "related", newContent, false)
	require.NoError(t, err)
	expectedChecksum := ContentChecksum(newContent)
	assert.Contains(t, string(result), "<!-- checksum:"+expectedChecksum+" -->")
}

func TestReplaceRegion_ChecksumMismatch(t *testing.T) {
	checksum := ContentChecksum([]byte("original content\n"))
	handEdited := []byte("<!-- VAULTMIND:GENERATED:related:START -->\n<!-- checksum:" + checksum + " -->\nhand edited content\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	_, err := ReplaceRegion(handEdited, "related", []byte("new\n"), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum_mismatch")
}

func TestReplaceRegion_ChecksumMismatch_Force(t *testing.T) {
	checksum := ContentChecksum([]byte("original content\n"))
	handEdited := []byte("<!-- VAULTMIND:GENERATED:related:START -->\n<!-- checksum:" + checksum + " -->\nhand edited content\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	result, err := ReplaceRegion(handEdited, "related", []byte("forced new\n"), true)
	require.NoError(t, err)
	assert.Contains(t, string(result), "forced new")
}

func TestReplaceRegion_NoChecksum_FirstTime(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\nplaceholder\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	result, err := ReplaceRegion(raw, "related", []byte("generated\n"), false)
	require.NoError(t, err)
	assert.Contains(t, string(result), "generated")
	assert.Contains(t, string(result), "<!-- checksum:")
}

func TestReplaceRegion_SectionNotFound(t *testing.T) {
	raw := []byte("# No markers here\n")
	_, err := ReplaceRegion(raw, "related", []byte("content\n"), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReplaceRegion_PreservesOtherMarkers(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\nold1\n<!-- VAULTMIND:GENERATED:related:END -->\n\n<!-- VAULTMIND:GENERATED:backlinks:START -->\nold2\n<!-- VAULTMIND:GENERATED:backlinks:END -->\n")
	result, err := ReplaceRegion(raw, "related", []byte("new1\n"), false)
	require.NoError(t, err)
	assert.Contains(t, string(result), "new1")
	assert.Contains(t, string(result), "old2")
}

func TestReplaceRegion_ChecksumMatch_NoError(t *testing.T) {
	content := []byte("generated content\n")
	checksum := ContentChecksum(content)
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\n<!-- checksum:" + checksum + " -->\ngenerated content\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	result, err := ReplaceRegion(raw, "related", []byte("updated content\n"), false)
	require.NoError(t, err)
	assert.Contains(t, string(result), "updated content")
}
