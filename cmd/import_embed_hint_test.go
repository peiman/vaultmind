package cmd

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Importing into a vault that was never embedded left the new notes
// keyword-only, and nothing said so (found in the local-files acceptance,
// 2026-10-03). The import names the command that turns on semantic search.

func TestImport_IntoAVaultNeverEmbeddedSaysHowToEmbed(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)

	_, errOut, err := runRootCmd(t, "import", filepath.Join(repo, "docs", "alpha.md"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "never been embedded")
	assert.Contains(t, errOut.String(), "vaultmind index --embed --vault "+vault)
}

func TestImport_IntoAnEmbeddedVaultSaysNothingAboutEmbedding(t *testing.T) {
	vault, repo := miniLMVault(t), docsRepo(t)
	stubEmbedPass(t, &index.EmbedResult{Embedded: 1}, nil)

	_, errOut, err := runRootCmd(t, "import", filepath.Join(repo, "docs", "alpha.md"), "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, errOut.String(), "never been embedded")
}

func TestImport_ADryRunSaysNothingAboutEmbedding(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)

	_, errOut, err := runRootCmd(t, "import", filepath.Join(repo, "docs", "alpha.md"), "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.NotContains(t, errOut.String(), "never been embedded")
}
