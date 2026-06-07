package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linksBothEnvelope decodes the combined --both JSON envelope: one envelope
// whose result carries an "out" and an "in" payload. The structural
// discriminators (source_id/target_id and the per-link target_id/source_id)
// let a test detect a direction swap.
type linksBothEnvelope struct {
	Status string `json:"status"`
	Result struct {
		Out struct {
			SourceID string `json:"source_id"`
			Links    []struct {
				TargetID *string `json:"target_id"`
				EdgeType string  `json:"edge_type"`
			} `json:"links"`
		} `json:"out"`
		In struct {
			TargetID string `json:"target_id"`
			Links    []struct {
				SourceID string `json:"source_id"`
				EdgeType string `json:"edge_type"`
			} `json:"in_links"`
		} `json:"in"`
	} `json:"result"`
}

// linksOutEnvelope decodes a single-direction --out envelope.
type linksOutEnvelope struct {
	Result struct {
		SourceID string `json:"source_id"`
		Links    []struct {
			TargetID *string `json:"target_id"`
		} `json:"links"`
	} `json:"result"`
}

// linksInEnvelope decodes a single-direction --in envelope.
type linksInEnvelope struct {
	Result struct {
		TargetID string `json:"target_id"`
		Links    []struct {
			SourceID string `json:"source_id"`
		} `json:"links"`
	} `json:"result"`
}

// memory links --out surfaces the outbound wikilink/related_ids edge.
// This is the new home for the old `links out`. Asserts the STRUCTURAL
// discriminator (source_id == the queried note, a link whose target_id is
// proj-beta) so a direction swap — which also contains "proj-beta" via the
// fixture back-ref — fails.
func TestMemoryLinks_OutFindsOutboundReferences(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--vault", vault, "--json")
	require.NoError(t, err)

	var env linksOutEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "concept-alpha", env.Result.SourceID,
		"--out envelope's source_id must be the queried note")
	var found bool
	for _, l := range env.Result.Links {
		if l.TargetID != nil && *l.TargetID == "proj-beta" {
			found = true
		}
	}
	assert.True(t, found, "--out must surface a link whose target_id is proj-beta")
}

// memory links --in surfaces the inbound backlink (beta references alpha).
// Asserts the structural discriminator (target_id == the queried note, a link
// whose source_id is proj-beta) so a direction swap fails.
func TestMemoryLinks_InFindsInboundReferences(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--in", "--vault", vault, "--json")
	require.NoError(t, err)

	var env linksInEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "concept-alpha", env.Result.TargetID,
		"--in envelope's target_id must be the queried note")
	var found bool
	for _, l := range env.Result.Links {
		if l.SourceID == "proj-beta" {
			found = true
		}
	}
	assert.True(t, found, "--in must surface a backlink whose source_id is proj-beta")
}

// memory links with no direction flag defaults to --both: the WHOLE stdout
// must unmarshal as ONE envelope carrying both an "out" and an "in" payload,
// with out before in. Two concatenated envelopes (invalid JSON) must fail.
func TestMemoryLinks_DefaultBothShowsBothDirections(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--vault", vault, "--json")
	require.NoError(t, err)

	body := out.Bytes()
	// The whole stdout must be exactly ONE JSON value — two concatenated
	// envelopes would error here.
	require.True(t, json.Valid(body), "default --both must emit a single valid JSON envelope")
	var env linksBothEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "ok", env.Status)

	// out direction: source_id is the queried note, link points at proj-beta.
	assert.Equal(t, "concept-alpha", env.Result.Out.SourceID)
	var haveOut bool
	for _, l := range env.Result.Out.Links {
		if l.TargetID != nil && *l.TargetID == "proj-beta" {
			haveOut = true
		}
	}
	assert.True(t, haveOut, "combined envelope must carry the outbound edge to proj-beta")

	// in direction: target_id is the queried note, link comes from proj-beta.
	assert.Equal(t, "concept-alpha", env.Result.In.TargetID)
	var haveIn bool
	for _, l := range env.Result.In.Links {
		if l.SourceID == "proj-beta" {
			haveIn = true
		}
	}
	assert.True(t, haveIn, "combined envelope must carry the inbound backlink from proj-beta")

	// Pin out-then-in key order in the serialized payload.
	outIdx := strings.Index(out.String(), "\"out\"")
	inIdx := strings.Index(out.String(), "\"in\"")
	require.NotEqual(t, -1, outIdx)
	require.NotEqual(t, -1, inIdx)
	assert.Less(t, outIdx, inIdx, "combined payload must serialize out before in")
}

// memory links --both explicitly behaves like the default: ONE envelope with
// both directions.
func TestMemoryLinks_BothFlagShowsBothDirections(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--both", "--vault", vault, "--json")
	require.NoError(t, err)

	body := out.Bytes()
	require.True(t, json.Valid(body), "--both must emit a single valid JSON envelope")
	var env linksBothEnvelope
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "concept-alpha", env.Result.Out.SourceID)
	assert.Equal(t, "concept-alpha", env.Result.In.TargetID)
}

// memory links without an argument is a usage error.
func TestMemoryLinks_MissingArgErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "links", "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "usage")
}

// memory links --out --in is rejected: the direction flags are mutually
// exclusive (M3). Cobra errors before RunE runs.
func TestMemoryLinks_OutAndInMutuallyExclusive(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--in", "--vault", vault)
	require.Error(t, err)
	// Cobra's MarkFlagsMutuallyExclusive emits: "if any flags in the group
	// [out in both] are set none of the others can be ...".
	assert.Contains(t, err.Error(), "none of the others can be")
	assert.Contains(t, err.Error(), "out")
	assert.Contains(t, err.Error(), "in")
}

// memory links --edge-type filters the surfaced edges. The outbound edge
// alpha -> beta carries edge type "related" (frontmatter related_ids); filtering
// to a non-matching type must drop it, while filtering to the matching type
// keeps it. This guards that --edge-type is actually plumbed through.
func TestMemoryLinks_EdgeTypeFilters(t *testing.T) {
	vault := buildIndexedTestVault(t)

	// Discover the real edge type of the alpha -> beta outbound edge.
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Links []struct {
				TargetID *string `json:"target_id"`
				EdgeType string  `json:"edge_type"`
			} `json:"links"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	var betaEdgeType string
	for _, l := range env.Result.Links {
		if l.TargetID != nil && *l.TargetID == "proj-beta" {
			betaEdgeType = l.EdgeType
		}
	}
	require.NotEmpty(t, betaEdgeType, "fixture must have an alpha -> beta outbound edge")

	// Filtering to the matching edge type keeps the edge.
	keepOut, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--edge-type", betaEdgeType, "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, keepOut.String(), "proj-beta",
		"--edge-type matching the edge must keep it")

	// Filtering to a non-matching edge type drops the edge.
	dropOut, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--edge-type", "no-such-edge-type", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.NotContains(t, dropOut.String(), "proj-beta",
		"--edge-type with a non-matching type must filter the edge out")
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

// newDeprecatedAlias-built commands must be Hidden so they don't clutter help
// while still resolving. Concretely: the `recall` and `context-pack`
// subcommands under the VISIBLE `memory` parent must have Hidden==true (L3).
func TestDeprecatedAliases_AreHidden(t *testing.T) {
	var memoryCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "memory" {
			memoryCmd = c
			break
		}
	}
	require.NotNil(t, memoryCmd, "the visible 'memory' parent must exist")
	require.False(t, memoryCmd.Hidden, "the 'memory' parent itself must stay visible")

	for _, name := range []string{"recall", "context-pack"} {
		var sub *cobra.Command
		for _, c := range memoryCmd.Commands() {
			if c.Name() == name {
				sub = c
				break
			}
		}
		require.NotNilf(t, sub, "memory %s alias must be registered", name)
		assert.Truef(t, sub.Hidden, "memory %s is a deprecated alias and must be Hidden", name)
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

// buildLowConfidenceNeighborVault makes a vault where concept-a and concept-b
// share a RARE tag (high TF-IDF specificity, lifted above the 1.0 threshold by
// six decoy notes carrying a different common tag). The indexer writes a
// LOW-confidence tag_overlap edge a→b. So:
//   - high-confidence default (memory neighbors) DROPS concept-b
//   - low-confidence default (links neighbors) SURFACES concept-b
//
// That divergence is what the M1 back-compat fix protects: the merged
// neighbors engine must keep the alias's old low/200 defaults.
func buildLowConfidenceNeighborVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  concept:\n    required: [title]\n    optional: [tags]\n"), 0o644))
	writeTestNote(t, dir, "a.md", "---\nid: concept-a\ntype: concept\ntitle: A\ntags: [rare]\n---\nBody A.\n")
	writeTestNote(t, dir, "b.md", "---\nid: concept-b\ntype: concept\ntitle: B\ntags: [rare]\n---\nBody B.\n")
	for i := 0; i < 6; i++ {
		writeTestNote(t, dir, fmt.Sprintf("d%d.md", i),
			fmt.Sprintf("---\nid: decoy-%d\ntype: concept\ntitle: D%d\ntags: [common]\n---\nBody.\n", i, i))
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err, "indexer rebuild failed")
	return dir
}

// The deprecated `links neighbors` alias must preserve its OLD defaults
// (min-confidence low, max-nodes 200) even though it now delegates to the
// merged `memory neighbors` engine whose defaults are high/50. A low-confidence
// tag_overlap neighbor that the high default would drop must still surface (M1).
func TestDeprecated_LinksNeighbors_PreservesLowConfidenceDefault(t *testing.T) {
	vault := buildLowConfidenceNeighborVault(t)

	// Canonical memory neighbors (high default) drops the low-confidence edge.
	high, _, err := runRootCmd(t, "memory", "neighbors", "concept-a",
		"--vault", vault, "--json")
	require.NoError(t, err)
	assert.NotContains(t, high.String(), "concept-b",
		"memory neighbors' high default must drop the low-confidence neighbor")

	// Deprecated links neighbors (low default) still surfaces it.
	low, _, err := runRootCmd(t, "links", "neighbors", "concept-a",
		"--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, low.String(), "concept-b",
		"links neighbors must keep its low-confidence default and surface the neighbor")
	assert.Contains(t, low.String(), "tag_overlap",
		"the surfaced edge must be the low-confidence tag_overlap edge")
}

// The links-neighbors alias preserves the old max-nodes default (200), not the
// canonical 50, when the user does not override it.
func TestDeprecated_LinksNeighbors_PreservesMaxNodesDefault(t *testing.T) {
	vault := buildIndexedTestVault(t)
	low, _, err := runRootCmd(t, "links", "neighbors", "concept-alpha",
		"--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			MaxNodes int `json:"max_nodes"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(low.Bytes(), &env))
	assert.Equal(t, 200, env.Result.MaxNodes,
		"links neighbors must keep the old max-nodes default of 200")
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
