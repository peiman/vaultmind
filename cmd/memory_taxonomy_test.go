package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memory links --out surfaces the outbound wikilink/related_ids edge.
// This is the new home for the old `links out`.
func TestMemoryLinks_OutFindsOutboundReferences(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "proj-beta",
		"--out must surface the outbound edge alpha -> beta")
}

// memory links --in surfaces the inbound backlink (beta references alpha).
func TestMemoryLinks_InFindsInboundReferences(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--in", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "proj-beta",
		"--in must surface the backlink beta -> alpha")
}

// memory links with no direction flag defaults to --both: it runs out then in,
// so both directions appear. Default must not be empty.
func TestMemoryLinks_DefaultBothShowsBothDirections(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--vault", vault, "--json")
	require.NoError(t, err)
	// Both directions reference proj-beta in this vault; the contract we lock
	// is that default produces two JSON envelopes (out + in), not zero.
	body := out.String()
	assert.GreaterOrEqual(t, strings.Count(body, "\"status\""), 2,
		"default --both must emit both an outbound and an inbound envelope")
	assert.Contains(t, body, "proj-beta")
}

// memory links --both explicitly behaves like the default.
func TestMemoryLinks_BothFlagShowsBothDirections(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--both", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, strings.Count(out.String(), "\"status\""), 2)
}

// memory links without an argument is a usage error.
func TestMemoryLinks_MissingArgErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "links", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// memory neighbors places the target at depth 0 and a 1-hop neighbor at
// depth 1, and (unlike the old links neighbors) carries full frontmatter
// (type + title) — that's the merge of recall + links neighbors.
func TestMemoryNeighbors_TargetAndNeighborWithFrontmatter(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "neighbors", "concept-alpha",
		"--vault", vault, "--depth", "1", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			Nodes []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Title    string `json:"title"`
				Distance int    `json:"distance"`
			} `json:"nodes"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))

	var haveTarget, haveNeighbor, haveFrontmatter bool
	for _, n := range env.Result.Nodes {
		if n.ID == "concept-alpha" && n.Distance == 0 {
			haveTarget = true
			if n.Type != "" && n.Title != "" {
				haveFrontmatter = true
			}
		}
		if n.Distance == 1 {
			haveNeighbor = true
		}
	}
	assert.True(t, haveTarget, "target must be at depth 0")
	assert.True(t, haveNeighbor, "at depth=1 there should be at least one neighbor")
	assert.True(t, haveFrontmatter, "neighbors must carry full frontmatter (type+title)")
}

// memory neighbors without an argument is a usage error.
func TestMemoryNeighbors_MissingArgErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "neighbors", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// memory pack includes the target — the rename of memory context-pack must
// preserve the protocol-level contract that the anchor note is always present.
func TestMemoryPack_IncludesTarget(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "pack", "concept-alpha",
		"--vault", vault, "--budget", "4000", "--max-items", "8", "--slim", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			TargetID string `json:"target_id"`
			Target   struct {
				ID string `json:"id"`
			} `json:"target"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "concept-alpha", env.Result.TargetID)
	assert.Equal(t, "concept-alpha", env.Result.Target.ID)
}

// memory pack human output reports the token budget line.
func TestMemoryPack_HumanOutputReportsBudget(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "pack", "concept-alpha",
		"--vault", vault, "--budget", "4000", "--max-items", "8", "--slim")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "tokens:")
	assert.Contains(t, out.String(), "4000")
}

// The top-level `links` command must no longer appear in the visible root
// listing: graph traversal lives under `memory` now. (It survives as a hidden
// parent so the deprecated subcommands still resolve.)
func TestTopLevelLinks_HiddenFromRootListing(t *testing.T) {
	for _, c := range RootCmd.Commands() {
		if c.Name() == "links" {
			assert.True(t, c.Hidden, "top-level 'links' must be hidden from the root listing")
			return
		}
	}
}

// ---- Deprecation aliases: must still run, delegate correctly, and warn. ----

// links out is a hidden deprecated alias of `memory links --out`: it prints a
// one-line stderr notice and still returns the outbound edge.
func TestDeprecated_LinksOut_WarnsAndDelegates(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "links", "out", "concept-alpha",
		"--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "proj-beta", "delegated output must still appear on stdout")
	assertOneLineDeprecation(t, errOut.String(), "memory links --out")
}

// links in is a hidden deprecated alias of `memory links --in`.
func TestDeprecated_LinksIn_WarnsAndDelegates(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "links", "in", "concept-alpha",
		"--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "proj-beta")
	assertOneLineDeprecation(t, errOut.String(), "memory links --in")
}

// links neighbors is a hidden deprecated alias of `memory neighbors`.
func TestDeprecated_LinksNeighbors_WarnsAndDelegates(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "links", "neighbors", "concept-alpha",
		"--vault", vault, "--depth", "1", "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "concept-alpha")
	assertOneLineDeprecation(t, errOut.String(), "memory neighbors")
}

// memory recall is a hidden deprecated alias of `memory neighbors`.
func TestDeprecated_MemoryRecall_WarnsAndDelegates(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "memory", "recall", "concept-alpha",
		"--vault", vault, "--depth", "1", "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "concept-alpha")
	assertOneLineDeprecation(t, errOut.String(), "memory neighbors")
}

// memory context-pack is a hidden deprecated alias of `memory pack`.
func TestDeprecated_MemoryContextPack_WarnsAndDelegates(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, errOut, err := runRootCmd(t, "memory", "context-pack", "concept-alpha",
		"--vault", vault, "--budget", "4000", "--max-items", "8", "--slim", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			TargetID string `json:"target_id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "concept-alpha", env.Result.TargetID, "delegation to memory pack must preserve target")
	assertOneLineDeprecation(t, errOut.String(), "memory pack")
}

// assertOneLineDeprecation checks that stderr carries exactly one non-empty
// deprecation line and that it names the new command path.
func assertOneLineDeprecation(t *testing.T, stderr, mustMention string) {
	t.Helper()
	lines := nonEmptyLines(stderr)
	require.Len(t, lines, 1, "deprecation notice must be exactly one line, got: %q", stderr)
	assert.Contains(t, lines[0], "deprecated")
	assert.Contains(t, lines[0], mustMention)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
