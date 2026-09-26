package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// search takes --vaults like ask does: one ranked section per vault. Search
// had only --vault, so knowledge spread over a project vault and a shared one
// had to be searched one vault at a time.

func TestSearch_VaultsSearchesEachVault(t *testing.T) {
	a := testvault.IndexedFixtureVault(t)
	b := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "search", "activation", "--vaults", a+","+b)
	require.NoError(t, err)
	assert.Contains(t, out.String(), a+" — ")
	assert.Contains(t, out.String(), b+" — ")
	assert.Contains(t, out.String(), "concept-spreading-activation")
}

func TestSearch_VaultsJSONHasOneResultPerVault(t *testing.T) {
	a := testvault.IndexedFixtureVault(t)
	b := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "search", "activation", "--vaults", a+","+b, "--json")
	require.NoError(t, err)
	var env struct {
		Status string `json:"status"`
		Result struct {
			Vaults []struct {
				Vault  string `json:"vault"`
				Result struct {
					Hits []struct {
						ID string `json:"id"`
					} `json:"hits"`
				} `json:"result"`
			} `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "ok", env.Status)
	require.Len(t, env.Result.Vaults, 2)
	assert.NotEmpty(t, env.Result.Vaults[0].Result.Hits)
}

func TestSearch_VaultsRejectsAPathThatIsNotAVault(t *testing.T) {
	a := testvault.IndexedFixtureVault(t)

	_, _, err := runRootCmd(t, "search", "activation", "--vaults", a+","+t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a vault")
}

// brokenVault is a vault by layout whose config cannot be loaded, so opening
// it fails after the not-a-vault check has passed.
func brokenVault(t *testing.T) string {
	t.Helper()
	v := testvault.IndexedFixtureVault(t)
	require.NoError(t, os.WriteFile(filepath.Join(v, ".vaultmind", "config.yaml"), []byte("types: [unclosed\n"), 0o600))
	return v
}

// A vault that fails to open fails the whole search with one error that names
// it — no half-printed sections, and in JSON one envelope, not a single-vault
// error envelope that drops the results already found.
func TestSearch_VaultsAFailingVaultIsOneNamedError(t *testing.T) {
	good := testvault.IndexedFixtureVault(t)
	bad := brokenVault(t)

	out, _, err := runRootCmd(t, "search", "activation", "--vaults", good+","+bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), bad)
	assert.Empty(t, out.String(), "nothing is printed before the error")

	out, _, _ = runRootCmd(t, "search", "activation", "--vaults", good+","+bad, "--json")
	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), "one JSON envelope: %s", out.String())
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Contains(t, env.Errors[0].Message, bad)
}
