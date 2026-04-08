package embedding_test

import (
	"os"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bgem3TestModelDir = "/tmp/vaultmind-bgem3-test/BAAI_bge-m3"

func TestLoadLinearWeights_SparseLinear(t *testing.T) {
	path := bgem3TestModelDir + "/sparse_linear.pt"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sparse_linear.pt not available at %s (download BGE-M3 model first)", path)
	}

	w, b, err := embedding.LoadLinearWeights(path)
	require.NoError(t, err, "should load sparse_linear.pt")

	// sparse_linear is Linear(1024, 1): weight [1][1024], bias [1]
	require.Len(t, w, 1, "output dim should be 1")
	assert.Len(t, w[0], 1024, "input dim should be 1024")
	require.Len(t, b, 1, "bias should have 1 element")
}

func TestLoadLinearWeights_ColBERTLinear(t *testing.T) {
	path := bgem3TestModelDir + "/colbert_linear.pt"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("colbert_linear.pt not available at %s", path)
	}

	w, b, err := embedding.LoadLinearWeights(path)
	require.NoError(t, err, "should load colbert_linear.pt")

	// colbert_linear is Linear(1024, 1024): weight [1024][1024], bias [1024]
	require.Len(t, w, 1024, "output dim should be 1024")
	assert.Len(t, w[0], 1024, "input dim should be 1024")
	require.Len(t, b, 1024, "bias should have 1024 elements")
}

func TestLoadLinearWeights_FileNotFound(t *testing.T) {
	_, _, err := embedding.LoadLinearWeights("/nonexistent/path.pt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "weight file not found")
}
