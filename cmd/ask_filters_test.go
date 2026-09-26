package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func askHitIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var env struct {
		Result struct {
			TopHits []struct {
				ID string `json:"id"`
			} `json:"top_hits"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), string(raw))
	ids := make([]string, len(env.Result.TopHits))
	for i, h := range env.Result.TopHits {
		ids[i] = h.ID
	}
	return ids
}

// In the baseline vault three notes mention "activation"; only
// g-semantic (semantic-network.md) is tagged "graph".
func TestAsk_TagScopesTheHits(t *testing.T) {
	vault := indexedBaselineVault(t)

	all, _, err := runRootCmd(t, "ask", "activation", "--vault", vault, "--json")
	require.NoError(t, err)
	require.Greater(t, len(askHitIDs(t, all.Bytes())), 1, "unscoped, several notes match")

	out, _, err := runRootCmd(t, "ask", "activation", "--vault", vault, "--json", "--tag", "graph")
	require.NoError(t, err)
	assert.Equal(t, []string{"g-semantic"}, askHitIDs(t, out.Bytes()))
}

func TestAsk_TypeScopesTheHits(t *testing.T) {
	vault := indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "ask", "activation", "--vault", vault, "--json", "--type", "concept")
	require.NoError(t, err)
	assert.NotEmpty(t, askHitIDs(t, out.Bytes()), "every baseline note is a concept")

	out, _, err = runRootCmd(t, "ask", "activation", "--vault", vault, "--json", "--type", "decision")
	require.NoError(t, err)
	assert.Empty(t, askHitIDs(t, out.Bytes()), "the baseline vault has no decisions")
}

func TestAsk_FiltersApplyToEveryFederatedVault(t *testing.T) {
	a, b := indexedBaselineVault(t), indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "ask", "activation", "--vaults", a+","+b, "--json", "--tag", "graph")
	require.NoError(t, err)
	ids := askHitIDs(t, out.Bytes())
	require.NotEmpty(t, ids, "the tagged note is in both vaults")
	for _, id := range ids {
		assert.Equal(t, "g-semantic", id, "a federated vault ignored the tag")
	}
}

// A federated vault holding no note with the tag reports that it had no hits,
// not that it has no embedder.
func TestAsk_FederatedVaultWithoutTheTagReportsNoHits(t *testing.T) {
	tagged, untagged := indexedBaselineVault(t), buildIndexedTestVault(t)

	out, _, err := runRootCmd(t, "ask", "activation", "--vaults", tagged+","+untagged, "--json", "--tag", "graph")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Vaults []struct {
				Name   string `json:"name"`
				NoHits bool   `json:"no_hits"`
			} `json:"federated_vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	require.Len(t, env.Result.Vaults, 2)
	noHits := map[string]bool{}
	for _, v := range env.Result.Vaults {
		noHits[v.Name] = v.NoHits
	}
	assert.False(t, noHits[vaultDisplayName(tagged)])
	assert.True(t, noHits[vaultDisplayName(untagged)])
}
