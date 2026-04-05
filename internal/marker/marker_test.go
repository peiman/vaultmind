package marker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindMarkers_NoMarkers(t *testing.T) {
	raw := []byte("# Title\n\nSome content.\n")
	markers, err := FindMarkers(raw)
	require.NoError(t, err)
	assert.Empty(t, markers)
}

func TestFindMarkers_SinglePair(t *testing.T) {
	raw := []byte("# Title\n\n<!-- VAULTMIND:GENERATED:related:START -->\nsome content\n<!-- VAULTMIND:GENERATED:related:END -->\n\nMore text.\n")
	markers, err := FindMarkers(raw)
	require.NoError(t, err)
	require.Len(t, markers, 1)
	assert.Equal(t, "related", markers[0].SectionKey)
	assert.Equal(t, "some content\n", markers[0].Content)
	assert.Empty(t, markers[0].Checksum)
}

func TestFindMarkers_WithChecksum(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\n<!-- checksum:abc123 -->\nquery content\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	markers, err := FindMarkers(raw)
	require.NoError(t, err)
	require.Len(t, markers, 1)
	assert.Equal(t, "abc123", markers[0].Checksum)
	assert.Equal(t, "query content\n", markers[0].Content)
}

func TestFindMarkers_TwoPairs(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\ncontent1\n<!-- VAULTMIND:GENERATED:related:END -->\n\n<!-- VAULTMIND:GENERATED:backlinks:START -->\ncontent2\n<!-- VAULTMIND:GENERATED:backlinks:END -->\n")
	markers, err := FindMarkers(raw)
	require.NoError(t, err)
	require.Len(t, markers, 2)
	assert.Equal(t, "related", markers[0].SectionKey)
	assert.Equal(t, "backlinks", markers[1].SectionKey)
}

func TestFindMarkers_UnpairedStart(t *testing.T) {
	raw := []byte("<!-- VAULTMIND:GENERATED:related:START -->\nsome content\n")
	_, err := FindMarkers(raw)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "related")
}

func TestFindMarkers_EndWithoutStart(t *testing.T) {
	raw := []byte("some content\n<!-- VAULTMIND:GENERATED:related:END -->\n")
	_, err := FindMarkers(raw)
	assert.Error(t, err)
}

func TestContentChecksum_Deterministic(t *testing.T) {
	content := []byte("some content\n")
	c1 := ContentChecksum(content)
	c2 := ContentChecksum(content)
	assert.Equal(t, c1, c2)
	assert.Len(t, c1, 64)
}

func TestContentChecksum_DifferentContent(t *testing.T) {
	c1 := ContentChecksum([]byte("content A\n"))
	c2 := ContentChecksum([]byte("content B\n"))
	assert.NotEqual(t, c1, c2)
}
