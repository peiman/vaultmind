package cmd

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A write embeds its note with the vault's own model. These tests pin the
// decisions around that pass: a vault never embedded is left alone, the opt-out
// holds, and BGE-M3 without the ORT backend is skipped with the command that
// finishes the job. The embedding pass itself loads a real model and is
// checked end to end on a real vault, not here.

// markEmbedded gives one note of the vault BGE-M3-shaped vectors, so the vault
// reads as embedded with BGE-M3.
func markEmbedded(t *testing.T, vault string) {
	t.Helper()
	db, err := index.Open(filepath.Join(vault, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`UPDATE notes SET embedding = x'00', sparse_embedding = x'00', colbert_embedding = x'00' WHERE path = 'concepts/alpha.md'`)
	require.NoError(t, err)
}

func TestEmbedOnWrite_AVaultNeverEmbeddedIsLeftAlone(t *testing.T) {
	vault := buildIndexedTestVault(t)

	_, errOut, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status", "paused", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, errOut.String(), "embed", "nothing to say when the vault has no embeddings")
}

func TestEmbedOnWrite_BGEM3WithoutORTSaysHowToFinish(t *testing.T) {
	if embedding.BackendName() == embedding.BackendNameORT {
		t.Skip("this build has the ORT backend; the skip path is for builds without it")
	}
	vault := buildIndexedTestVault(t)
	markEmbedded(t, vault)

	_, errOut, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status", "paused", "--vault", vault)
	require.NoError(t, err, "the write itself succeeds")
	assert.Contains(t, errOut.String(), "not embedded")
	assert.Contains(t, errOut.String(), "vaultmind index --embed --vault "+vault)
}

func TestEmbedOnWrite_CanBeTurnedOff(t *testing.T) {
	vault := buildIndexedTestVault(t)
	markEmbedded(t, vault)
	viper.Set(config.KeyAppEmbedOnWrite, false)
	t.Cleanup(func() { viper.Set(config.KeyAppEmbedOnWrite, true) })

	_, errOut, err := runRootCmd(t, "frontmatter", "set", "projects/beta.md", "status", "paused", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, errOut.String(), "embed", "turned off, it neither embeds nor explains")
}
