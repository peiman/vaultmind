package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeEmbedding_RoundTrip(t *testing.T) {
	original := []float32{0.1, -0.5, 3.14, 0, -1e-6}
	encoded := index.EncodeEmbedding(original)
	assert.Len(t, encoded, len(original)*4, "each float32 is 4 bytes")

	decoded, err := index.DecodeEmbedding(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeEmbedding_InvalidLength(t *testing.T) {
	_, err := index.DecodeEmbedding([]byte{0x01, 0x02, 0x03}) // 3 bytes, not divisible by 4
	assert.Error(t, err)
}

func TestEncodeEmbedding_Empty(t *testing.T) {
	encoded := index.EncodeEmbedding(nil)
	assert.Nil(t, encoded)
}

func TestDecodeEmbedding_Empty(t *testing.T) {
	decoded, err := index.DecodeEmbedding(nil)
	require.NoError(t, err)
	assert.Nil(t, decoded)
}
