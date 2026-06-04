package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_IndexRetrieveAskSmoke is the inter-layer contract test that the
// 2026-04-24 ranking bug would have caught pre-emptively if it existed.
// Individual layers (indexer, retriever, ask, format) each had tests; the
// bug lived *between* them — partial modality writes that the indexer
// produced + the retriever silently accepted + ask silently surfaced as a
// wrong top hit. A smoke test that runs the full pipeline on a known
// fixture and asserts on the user-visible envelope is the only place that
// class of bug shows up.
//
// Scope: the committed fixture vault + FTS-only retrieval (no embeddings).
// Hybrid and dense paths have their own unit tests; indexing with MiniLM
// adds an external model download and is gated elsewhere behind
// VAULTMIND_TEST_EMBEDDING. This test stays hermetic and fast by design.
//
// What it locks in:
//   - `ask <query> --json` produces a v1 envelope with status=ok
//   - The envelope has a non-empty TopHits array
//   - The top hit's ID is the one a literate reader would predict from the
//     query text — a regression would mean the FTS → retriever → ask chain
//     is mis-wired at a layer boundary, which is exactly the class of
//     failure we want CI to flag
func TestE2E_IndexRetrieveAskSmoke(t *testing.T) {
	vault := buildIndexedTestVault(t)

	// Case 1: query that matches the Alpha note's title + body.
	out, _, err := runRootCmd(t, "ask", "Alpha Concept",
		"--vault", vault,
		"--json",
		"--budget", "1500", "--max-items", "3", "--search-limit", "5")
	require.NoError(t, err, "ask must succeed on a literal-match query against a committed fixture")

	var env struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Result        struct {
			Query   string `json:"query"`
			TopHits []struct {
				ID string `json:"id"`
			} `json:"top_hits"`
			RetrievalMode string `json:"retrieval_mode"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), "envelope must decode cleanly")

	assert.Equal(t, "v1", env.SchemaVersion, "schema version is a PUBLIC contract — consumers branch on it")
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "Alpha Concept", env.Result.Query, "query must echo — losing it loses provenance")
	require.NotEmpty(t, env.Result.TopHits, "literal-match query must return at least one hit")
	assert.Equal(t, "concept-alpha", env.Result.TopHits[0].ID,
		"FTS → retriever → ask pipeline must surface the literal-match target at rank 1")

	// Case 2: different query, different expected target — proves the
	// plumbing routes per-query, not by accident.
	out2, _, err := runRootCmd(t, "ask", "Beta Project",
		"--vault", vault,
		"--json",
		"--budget", "1500", "--max-items", "3", "--search-limit", "5")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out2.Bytes(), &env))
	require.NotEmpty(t, env.Result.TopHits)
	assert.Equal(t, "proj-beta", env.Result.TopHits[0].ID,
		"second-query routing must be independent — a broken cache or shared-state bug shows here")
}

// TestE2E_AskExplainFlagIsAccepted locks in --explain's CLI plumbing: the
// flag parses, the config key resolves, the formatter dispatch routes to
// FormatAskExplain. Lane-breakdown rendering is tested in
// internal/query/runners_test.go with a HybridRetriever result — this
// fixture has no embeddings, so the auto-retriever falls back to FTS which
// is a single-lane retriever and by design emits no per-lane components.
// The integration invariant here is narrower: the flag is wired end-to-end
// and ask still succeeds.
func TestE2E_AskExplainFlagIsAccepted(t *testing.T) {
	vault := buildIndexedTestVault(t)

	out, _, err := runRootCmd(t, "ask", "Alpha Concept",
		"--vault", vault,
		"--explain",
		"--budget", "1500", "--max-items", "3", "--search-limit", "5")
	require.NoError(t, err, "--explain flag must parse and the command must succeed")

	text := out.String()
	assert.Contains(t, text, "Search:",
		"formatter dispatch must still render the header — a regression here means the flag broke the default path")
	assert.Contains(t, text, "concept-alpha",
		"the hit must still appear — --explain is additive, it must not suppress existing output")
}
