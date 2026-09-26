package cmd

import (
	"encoding/json"
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
