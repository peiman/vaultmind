package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
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

func TestImport_ManySourcesSayHowToEmbedOncePerCall(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := docsRepo(t)
	for _, jsonMode := range []bool{false, true} {
		t.Run(fmt.Sprint(jsonMode), func(t *testing.T) {
			vault := indexedBaselineVault(t)
			// A failure before the changed sources must not consume the hint.
			args := []string{"import", filepath.Join(repo, "missing.md"), filepath.Join(repo, "docs", "alpha.md"), filepath.Join(repo, "docs", "beta.md"), "--vault", vault, "--force"}
			if jsonMode {
				args = append(args, "--json")
			}
			_, errOut, err := runRootCmd(t, args...)
			require.Error(t, err)
			assert.Equal(t, 1, strings.Count(errOut.String(), "never been embedded"))
		})
	}
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
