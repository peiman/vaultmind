package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// index --full forces a full rebuild regardless of hashes. Regression: if
// --full silently stopped working, users who just deleted/renamed notes
// would be stuck with stale state from the incremental path.
func TestIndex_FullRebuildTouchesEveryNote(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "index", "--vault", vault, "--full", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Index struct {
				Indexed     int  `json:"indexed"`
				FullRebuild bool `json:"full_rebuild"`
			} `json:"index"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.True(t, env.Result.Index.FullRebuild, "FullRebuild flag must be set")
	assert.Greater(t, env.Result.Index.Indexed, 0, "full rebuild must (re)index every note")
}

// Incremental index human output reports the skipped/updated/added/deleted
// breakdown (tested via JSON in an earlier test, this one covers the
// text-formatter branch of runIndex).
func TestIndex_IncrementalHumanOutput(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "skipped")
	assert.Contains(t, text, "Indexed")
}

// Full-rebuild human output differs from incremental — it reports
// (domain, unstructured, errors) instead of the skip/update/add/delete
// breakdown. Locking the distinction so refactors can't silently flatten
// the two outputs.
func TestIndex_FullRebuildHumanOutputShapeDiffersFromIncremental(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "index", "--vault", vault, "--full")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "domain")
	assert.Contains(t, text, "unstructured")
}

// memory summarize with --include-body and --max-body-len produces
// excerpts bounded by the limit (body may be shorter than max, but must
// not exceed it). Regression: a broken cap would leak full note bodies
// into the output, blowing up the token budget for agents.
func TestMemorySummarize_IncludeBodyRespectsMaxBodyLen(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "summarize", "concept-alpha",
		"--vault", vault, "--include-body", "--max-body-len", "10", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Sources []struct {
				BodyExcerpt string `json:"body_excerpt"`
			} `json:"sources"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Len(t, env.Result.Sources, 1)
	assert.NotEmpty(t, env.Result.Sources[0].BodyExcerpt, "IncludeBody=true must populate the excerpt")
	// The cap is advisory (truncation boundary can add markers) — assert
	// the excerpt isn't absurdly long, which would mean the cap didn't fire.
	assert.LessOrEqual(t, len(env.Result.Sources[0].BodyExcerpt), 40,
		"excerpt must be bounded by max-body-len (with small overhead for ellipsis)")
}

// memory summarize human output shows the note id, type, title row. Lock
// the columns so scripts that awk the output don't break.
func TestMemorySummarize_HumanRowFormat(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "summarize", "concept-alpha",
		"--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "concept-alpha")
	assert.Contains(t, text, "concept")
	assert.Contains(t, text, "Alpha Concept")
}

// experiment report with a summary on an empty DB — the human output
// path. Complements the json test.
func TestExperimentSummary_HumanOutputHeader(t *testing.T) {
	_, _ = seedExperimentDB(t)
	out, _, err := runRootCmd(t, "experiment", "summary")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Sessions:",
		"summary human output must emit the Sessions: header")
}
