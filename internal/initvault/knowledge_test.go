package initvault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A knowledge vault is a project's knowledge base: decisions, concepts, the
// code they are about. It carries none of the persona scaffold — a stranger
// who ran init for a docs vault got identity/, arcs/ and principles/.
func TestInitKnowledge_ScaffoldsAProjectKnowledgeBase(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "docs-kb")

	res, err := initvault.InitKnowledge(dst)
	require.NoError(t, err)
	assert.Equal(t, dst, res.VaultPath)

	for _, want := range []string{"README.md", ".vaultmind/config.yaml", "decisions/decision-example.md", "concepts/concept-example.md"} {
		assert.FileExists(t, filepath.Join(dst, want))
	}
	for _, persona := range []string{"identity", "arcs", "principles"} {
		assert.NoDirExists(t, filepath.Join(dst, persona))
	}
	assert.Equal(t, 4, res.FilesAdded)

	readme, err := os.ReadFile(filepath.Join(dst, "README.md")) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Contains(t, string(readme), "paths:", "the README teaches tying a note to code")
	assert.NotContains(t, string(readme), "identity/")
}

// One type registry for both scaffolds: the knowledge vault reads the same
// config the persona vault does, so a type added once exists in both.
func TestInitKnowledge_SharesTheTypeRegistry(t *testing.T) {
	kb, persona := filepath.Join(t.TempDir(), "kb"), filepath.Join(t.TempDir(), "p")
	_, err := initvault.InitKnowledge(kb)
	require.NoError(t, err)
	_, err = initvault.Init(persona)
	require.NoError(t, err)

	a, err := os.ReadFile(filepath.Join(kb, filepath.FromSlash(vault.ConfigRelPath))) //nolint:gosec // test path
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(persona, filepath.FromSlash(vault.ConfigRelPath))) //nolint:gosec // test path
	require.NoError(t, err)
	assert.Equal(t, string(b), string(a))
	_, err = vault.LoadConfig(kb)
	assert.NoError(t, err)
}

// The examples show paths: without matching real code: a live pattern in an
// example note would surface it on every read of that code.
func TestInitKnowledge_ExamplesDoNotCoverRealCode(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "kb")
	_, err := initvault.InitKnowledge(dst)
	require.NoError(t, err)
	for _, rel := range []string{"decisions/decision-example.md", "concepts/concept-example.md"} {
		body, err := os.ReadFile(filepath.Join(dst, rel)) //nolint:gosec // test path
		require.NoError(t, err)
		assert.Contains(t, string(body), "# paths:", "%s shows the field, commented out", rel)
		assert.NotRegexp(t, `(?m)^paths:`, string(body), "%s must not cover real code", rel)
	}
}

func TestInitKnowledge_RefusesExistingPath(t *testing.T) {
	_, err := initvault.InitKnowledge(t.TempDir())
	require.Error(t, err)
}
