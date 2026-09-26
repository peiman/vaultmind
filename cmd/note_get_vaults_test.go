package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A federated ask ranks ids from every vault but names one vault in its
// footer. note get --vaults reads an id from whichever listed vault holds it.
func TestNoteGet_VaultsReadsFromTheVaultHoldingTheID(t *testing.T) {
	testVault, baseline := buildIndexedTestVault(t), indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "note", "get", "g-semantic", "--vaults", testVault+","+baseline)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Semantic networks represent concepts", "the body comes from the second vault")

	out, _, err = runRootCmd(t, "note", "get", "concept-alpha", "--vaults", testVault+","+baseline, "--json")
	require.NoError(t, err)
	var env struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "ok", env.Status)
}

func TestNoteGet_VaultsNamesEveryVaultSearchedWhenNothingMatches(t *testing.T) {
	testVault, baseline := buildIndexedTestVault(t), indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "note", "get", "no-such-note", "--vaults", testVault+","+baseline)
	require.Error(t, err, "a miss fails, as it does for one vault")
	assert.Contains(t, out.String(), vaultDisplayName(testVault))
	assert.Contains(t, out.String(), vaultDisplayName(baseline))
}

// The same id in two vaults is not resolved by picking one: that is the
// silent choice that made --vault A --vault B answer from B alone.
func TestNoteGet_VaultsRefusesAnIDHeldByTwoVaults(t *testing.T) {
	a, b := indexedBaselineVault(t), indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "note", "get", "g-semantic", "--vaults", a+","+b, "--json")
	require.Error(t, err)
	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code       string   `json:"code"`
			Candidates []string `json:"candidates"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "ambiguous_vault", env.Errors[0].Code)
	assert.ElementsMatch(t, []string{a, b}, env.Errors[0].Candidates)
}

func TestNoteGet_VaultsRejectsAPathThatIsNotAVault(t *testing.T) {
	baseline := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "note", "get", "concept-alpha", "--vaults", baseline+","+t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a vault")
}

func TestNoteGet_VaultsTextErrorNamesBothHolders(t *testing.T) {
	a, b := indexedBaselineVault(t), indexedBaselineVault(t)

	_, _, err := runRootCmd(t, "note", "get", "g-semantic", "--vaults", a+","+b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), a)
	assert.Contains(t, err.Error(), b)
	assert.Contains(t, err.Error(), "--vault <one of them>")
}

func TestNoteGet_VaultsJSONMissIsNotFound(t *testing.T) {
	testVault, baseline := buildIndexedTestVault(t), indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "note", "get", "no-such-note", "--vaults", testVault+","+baseline, "--json")
	require.Error(t, err)
	var env struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	require.Len(t, env.Errors, 1)
	assert.Equal(t, "not_found", env.Errors[0].Code)
	assert.Contains(t, env.Errors[0].Message, vaultDisplayName(baseline))
}
