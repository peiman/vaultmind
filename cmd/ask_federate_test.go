package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Multi-vault was not merely unsupported — it was silently wrong. `--vault A
// --vault B` kept B and answered "nothing relevant" without ever opening A.
// These cover the parsing half: which vaults a run will actually search.

func TestResolveAskVaultPaths_SingleVaultUnchanged(t *testing.T) {
	got, err := resolveAskVaultPaths("/a/identity", "")
	require.NoError(t, err)
	require.Equal(t, []string{"/a/identity"}, got, "the single-vault path must not change shape")
}

func TestResolveAskVaultPaths_CommaSeparatedFederates(t *testing.T) {
	got, err := resolveAskVaultPaths("/a/identity", "/a/identity,/a/desk,/a/research")
	require.NoError(t, err)
	require.Equal(t, []string{"/a/identity", "/a/desk", "/a/research"}, got)
}

func TestResolveAskVaultPaths_TrimsBlanksAndDeduplicates(t *testing.T) {
	got, err := resolveAskVaultPaths("/a/identity", " /a/desk , , /a/desk ,/a/research")
	require.NoError(t, err)
	require.Equal(t, []string{"/a/desk", "/a/research"}, got,
		"a repeated vault would double-count in RRF and silently outrank the others")
}

func TestResolveAskVaultPaths_OnlyBlanksIsAnError(t *testing.T) {
	_, err := resolveAskVaultPaths("/a/identity", " , ")
	require.Error(t, err, "asking to federate across nothing must not quietly fall back to one vault")
}

func TestVaultDisplayName_IsTheDirectoryName(t *testing.T) {
	require.Equal(t, "vaultmind-mine", vaultDisplayName("/Users/x/dev/vaultmind-mine"))
	require.Equal(t, "vaultmind-mine", vaultDisplayName("/Users/x/dev/vaultmind-mine/"))
}
