package cmd

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `vaultmind ask <query> --read N` resolves to the Nth-ranked hit's
// body and prints it inline with the menu. Pins the rank-form path of
// the round-2-derived shortcut: probe + read in a single command.
func TestAsk_ReadByRankRendersChosenBody(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault, "--read", "1")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "concept-alpha", "menu must include the chosen hit's id")
	assert.Contains(t, text, "Alpha", "chosen note's title must surface")
}

// --read by id resolves an exact id from the returned hits. Mirrors the
// agent workflow: see the menu, pick by id, get the body — no copy-paste.
func TestAsk_ReadByIDRendersChosenBody(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault, "--read", "concept-alpha")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "concept-alpha")
}

// --read with an out-of-range rank errors clearly. Bad ranks are likely
// typos; the error message names the valid range so the agent fixes it
// in one step rather than guessing.
func TestAsk_ReadOutOfRangeRankErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault, "--read", "99")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only", "error must name the available range")
	assert.Contains(t, err.Error(), "hit", "error mentions hits not some other counter")
}

// --read with an id not in the returned hits errors with both recovery
// paths named (re-run without --read OR use note get directly). This
// is the "model-quality error message" the round-3 evaluator
// specifically called out as worth keeping.
func TestAsk_ReadUnknownIDErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault, "--read", "concept-not-in-results")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "id not in returned hits", "error must name the failure mode")
	assert.Contains(t, msg, "note get", "error must surface the direct-lookup recovery")
}

// resolveAskReadTarget handles the empty-hits case explicitly — caller
// shouldn't have to special-case "search returned nothing."
func TestResolveAskReadTarget_EmptyHitsErrors(t *testing.T) {
	_, err := resolveAskReadTarget(nil, "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no search hits")
}

// resolveAskReadTarget by rank — happy path, 1-indexed.
func TestResolveAskReadTarget_RankResolves(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	chosen, err := resolveAskReadTarget(hits, "2")
	require.NoError(t, err)
	assert.Equal(t, "b", chosen.ID, "rank 2 maps to hits[1] (1-indexed)")
}

// resolveAskReadTarget by id — happy path, exact match.
func TestResolveAskReadTarget_IDResolves(t *testing.T) {
	hits := []retrieval.ScoredResult{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	chosen, err := resolveAskReadTarget(hits, "c")
	require.NoError(t, err)
	assert.Equal(t, "c", chosen.ID)
}

// `--read N --json` errors loudly rather than emit a partial envelope.
// The default JSON shape doesn't naturally carry "the user chose this
// rank"; punted until a real consumer asks. Pin the punt so a future
// silent-emit doesn't sneak in.
func TestAsk_ReadWithJSONErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault, "--read", "1", "--json")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "--json") || strings.Contains(err.Error(), "JSON"),
		"error must name the --json incompatibility")
}
