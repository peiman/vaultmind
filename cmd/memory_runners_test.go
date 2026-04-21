package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memory recall must return the target at depth 0, plus anything at deeper
// distances up to --depth. Regression: depth=0 elided (target invisible) or
// depth exceeded silently.
func TestMemoryRecall_TargetAtDepthZero(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "recall", "concept-alpha",
		"--vault", vault, "--depth", "1", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Nodes []struct {
				ID       string `json:"id"`
				Distance int    `json:"distance"`
			} `json:"nodes"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))

	// Target must appear at depth 0 so the caller always knows *which* note
	// the recall started from.
	var gotTarget bool
	for _, n := range env.Result.Nodes {
		if n.ID == "concept-alpha" && n.Distance == 0 {
			gotTarget = true
		}
	}
	assert.True(t, gotTarget, "target concept-alpha must appear at depth 0")
}

// memory recall without an argument must usage-error — silent success would
// mask script bugs.
func TestMemoryRecall_MissingArgErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "recall", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// memory recall human output labels the target as "depth 0" — the visual
// distinction between the focal note and its neighbors is what makes the
// output readable.
func TestMemoryRecall_HumanOutputLabelsDepthZero(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "recall", "concept-alpha",
		"--vault", vault, "--depth", "1")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "(depth 0)")
	assert.Contains(t, out.String(), "concept-alpha")
}

// memory related must surface related_ids as related items. If the frontmatter
// related_ids edge is dropped, downstream exploration UIs go blind.
func TestMemoryRelated_SurfacesRelatedIDs(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "related", "concept-alpha",
		"--vault", vault, "--json")
	require.NoError(t, err)

	// alpha declares related_ids: [proj-beta]; related should include it.
	assert.Contains(t, out.String(), "proj-beta")
}

// memory context-pack must include the target in its payload — a context
// pack missing the target is a protocol-level bug that would starve the
// consuming agent of its anchor.
func TestMemoryContextPack_IncludesTarget(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "context-pack", "concept-alpha",
		"--vault", vault, "--budget", "4000", "--max-items", "8", "--slim", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			TargetID string `json:"target_id"`
			Target   struct {
				ID string `json:"id"`
			} `json:"target"`
			Context []struct {
				ID string `json:"id"`
			} `json:"context"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "concept-alpha", env.Result.TargetID)
	assert.Equal(t, "concept-alpha", env.Result.Target.ID)
}

// memory context-pack human output reports a token budget line. Regression:
// dropping it would make "am I close to budget?" invisible to the user.
func TestMemoryContextPack_HumanOutputReportsBudget(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "context-pack", "concept-alpha",
		"--vault", vault, "--budget", "4000", "--max-items", "8", "--slim")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "tokens:")
	assert.Contains(t, out.String(), "4000", "configured budget must appear in output")
}

// memory summarize must partition requested IDs into found+NotFound. Losing
// the NotFound half would silently skip typos the caller needs to surface.
func TestMemorySummarize_PartitionsFoundAndNotFound(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "summarize",
		"concept-alpha", "does-not-exist", "proj-beta",
		"--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Sources []struct {
				ID string `json:"id"`
			} `json:"sources"`
			NotFound []string `json:"not_found"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Len(t, env.Result.Sources, 2)
	assert.Contains(t, env.Result.NotFound, "does-not-exist")
}

// memory summarize with no args and no --ids must usage-error. A silent
// no-op would look like "success, zero notes" which is wrong.
func TestMemorySummarize_NoIDsErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "summarize", "--vault", vault)
	require.Error(t, err)
}

// collectSummarizeIDs: args win if both args and --ids are present — but when
// no args are supplied, --ids must be split and trimmed just like note mget.
// This test locks the unit-level contract for the helper.
func TestCollectSummarizeIDs_ArgsWinOverIDsFlag(t *testing.T) {
	cmd := memorySummarizeCmd
	require.NoError(t, cmd.Flags().Set("ids", "from-flag-a,from-flag-b"))
	// args present → args win
	got := collectSummarizeIDs(cmd, []string{"arg-1"})
	assert.Equal(t, []string{"arg-1"}, got)

	// no args → --ids used, trimmed+split
	got = collectSummarizeIDs(cmd, nil)
	assert.Equal(t, []string{"from-flag-a", "from-flag-b"}, got)

	// reset
	require.NoError(t, cmd.Flags().Set("ids", ""))
	assert.Empty(t, collectSummarizeIDs(cmd, nil))
}
