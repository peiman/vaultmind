package vault_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalKey_ExistingPathAndItsSymlinkMatch(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.Symlink(real, link))

	want, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, want, vault.CanonicalKey(real))
	assert.Equal(t, want, vault.CanonicalKey(link))
}

func TestCanonicalKey_RelativeDotIsTheWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(wd)
	require.NoError(t, err)
	assert.Equal(t, want, vault.CanonicalKey("."))
}

func TestCanonicalKey_MissingPathStaysAbsolute(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-yet")
	got := vault.CanonicalKey(missing)
	assert.True(t, filepath.IsAbs(got))
	assert.Equal(t, filepath.Clean(missing), got)
}
