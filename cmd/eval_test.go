package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baselineQueries = "../test/fixtures/baseline/queries.yaml"

// eval scores a vault's retrieval against a labelled query set — the same
// Hit@K and MRR the golden baseline test uses, run on any vault. Without it a
// ranking change could only be judged by whether results moved, not whether
// they got better.
func TestEval_ScoresAVaultAgainstLabelledQueries(t *testing.T) {
	vault := indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "eval", baselineQueries, "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	for _, want := range []string{"queries against", "Hit@1", "Hit@5", "MRR"} {
		assert.Contains(t, text, want)
	}
}

func TestEval_JSONCarriesEveryQueryAndItsRank(t *testing.T) {
	vault := indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "eval", baselineQueries, "--vault", vault, "--json", "--k", "3")
	require.NoError(t, err)
	var env struct {
		Result struct {
			K       int     `json:"k"`
			Queries int     `json:"query_count"`
			HitAt1  float64 `json:"hit_at_1"`
			HitAtK  float64 `json:"hit_at_k"`
			MRR     float64 `json:"mrr"`
			PerQ    []struct {
				Name string `json:"name"`
				Rank int    `json:"rank"`
			} `json:"queries"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	r := env.Result
	assert.Equal(t, 3, r.K)
	require.NotZero(t, r.Queries)
	assert.Len(t, r.PerQ, r.Queries)
	assert.GreaterOrEqual(t, r.HitAtK, r.HitAt1, "a hit at 1 is a hit at 3")
	assert.Positive(t, r.MRR, "the fixture's golden queries find their notes")
	for _, q := range r.PerQ {
		assert.GreaterOrEqual(t, q.Rank, 0, "%s: 0 means not found, otherwise the 1-based rank", q.Name)
	}
}

// A query whose relevant note is missing from the results is listed with the
// rank it got, so the reader sees what to look at.
func TestEval_TextListsQueriesNotAnsweredFirst(t *testing.T) {
	vault := indexedBaselineVault(t)
	queries := filepath.Join(t.TempDir(), "q.yaml")
	require.NoError(t, os.WriteFile(queries, []byte("- name: impossible\n  text: spreading activation\n  expected: [no-such-note]\n"), 0o600))

	out, _, err := runRootCmd(t, "eval", queries, "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "impossible")
	assert.Contains(t, out.String(), "not in the top")
}

func TestEval_RefusesAMissingOrEmptyQuerySet(t *testing.T) {
	vault := indexedBaselineVault(t)
	_, _, err := runRootCmd(t, "eval", filepath.Join(t.TempDir(), "absent.yaml"), "--vault", vault)
	require.Error(t, err)

	empty := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(empty, []byte("[]\n"), 0o600))
	_, _, err = runRootCmd(t, "eval", empty, "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is empty")
}
