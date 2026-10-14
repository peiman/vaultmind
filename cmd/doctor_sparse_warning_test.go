package cmd

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func staleReadyEmbeddings() *query.DoctorEmbeddings {
	return &query.DoctorEmbeddings{
		SemanticReady:     true,
		SparseFormatStale: true,
		Model:             embedding.ModelBGEM3,
		TotalNotes:        1,
		DenseCount:        1,
		SparseCount:       1,
		ColBERTCount:      1,
	}
}

func TestWriteEmbeddingStatus_StaleSparseUsesSharedRemedyAndVaultPath(t *testing.T) {
	vault := "/tmp/vault-a"
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, staleReadyEmbeddings(), &query.DoctorIssues{}, vault))
	out := buf.String()
	assert.Contains(t, out, "run: "+index.SparseFormatStaleRemedy+" --vault "+vault)
	assert.NotContains(t, out, "<vault>")
	assert.Contains(t, out, index.SparseFormatStaleNotice)
}

func TestWriteEmbeddingStatus_StaleSparseQuotesAPathWithSpaces(t *testing.T) {
	vault := "/tmp/my vault"
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, staleReadyEmbeddings(), &query.DoctorIssues{}, vault))
	assert.Contains(t, buf.String(), "run: "+index.SparseFormatStaleRemedy+" --vault '"+vault+"'")
}

func TestWriteEmbeddingStatus_StaleSparseOmitsCommandWhenIndexWouldRefuse(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, staleReadyEmbeddings(), &query.DoctorIssues{
		IndexRootBlocked: true,
		ResolvedRoot:     "/empty",
	}, "/tmp/vault-a"))
	out := buf.String()
	assert.Contains(t, out, index.SparseFormatStaleNotice)
	assert.NotContains(t, out, "run:")
}

func TestWriteEmbeddingStatus_StaleSparseWithoutPathUsesSharedRemedy(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeEmbeddingStatus(&buf, staleReadyEmbeddings(), nil, ""))
	out := buf.String()
	assert.Contains(t, out, "run: "+index.SparseFormatStaleRemedy+"\n")
	assert.NotContains(t, out, "<vault>")
	assert.NotContains(t, out, "--vault")
}
