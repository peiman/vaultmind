package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/marker"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// links neighbors must place the target at depth 0 and its 1-hop neighbors
// at depth 1, with edge attribution. Regression: losing edge attribution
// makes the traversal unreadable — the user can't tell *why* a neighbor is
// related.
func TestLinksNeighbors_ReturnsDepthAnnotatedNodes(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "links", "neighbors", "concept-alpha",
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

	var haveTarget, haveNeighbor bool
	for _, n := range env.Result.Nodes {
		if n.ID == "concept-alpha" && n.Distance == 0 {
			haveTarget = true
		}
		if n.Distance == 1 {
			haveNeighbor = true
		}
	}
	assert.True(t, haveTarget, "target must be at depth 0")
	assert.True(t, haveNeighbor, "at depth=1 there should be at least one neighbor")
}

// links neighbors without an argument is a usage error.
func TestLinksNeighbors_MissingArgErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "links", "neighbors", "--vault", vault)
	require.Error(t, err)
}

// formatNeighbors renders depth-0 target and depth>0 neighbors distinctly,
// and appends "(max reached)" only when the traversal hit its cap. The
// visual distinction is what makes the output scannable.
func TestFormatNeighbors_DistinguishesTargetAndMaxReached(t *testing.T) {
	r := &query.NeighborsResult{
		Nodes: []query.NeighborNode{
			{ID: "t-1", Distance: 0},
			{ID: "t-2", Distance: 1, EdgeFrom: &query.NeighborEdge{EdgeType: "related", Confidence: "high"}},
		},
		MaxNodesReached: true,
	}
	var buf bytes.Buffer
	require.NoError(t, formatNeighbors(r, &buf))
	out := buf.String()
	assert.Contains(t, out, "t-1 (depth 0)")
	assert.Contains(t, out, "t-2")
	assert.Contains(t, out, "related")
	assert.Contains(t, out, "(max reached)")
}

// note create without --type must fail with a clear usage error — the alt
// is creating a typeless note the registry cannot validate.
func TestNoteCreate_MissingTypeErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "note", "create", "concepts/fresh.md",
		"--vault", vault)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "type")
}

// note create (happy path through RootCmd) writes the file and returns an
// envelope with path+id. This test covers runNoteCreate which the existing
// internal-only tests skip by calling executeNoteCreate directly.
func TestNoteCreate_HappyPathViaRootCmd(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "note", "create", "concepts/fresh.md",
		"--type", "concept",
		"--field", "title=Fresh",
		"--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			Path    string `json:"path"`
			ID      string `json:"id"`
			Type    string `json:"type"`
			Created bool   `json:"created"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "concepts/fresh.md", env.Result.Path)
	assert.Equal(t, "concept", env.Result.Type)
	assert.True(t, env.Result.Created)

	_, statErr := os.Stat(filepath.Join(vault, "concepts/fresh.md"))
	require.NoError(t, statErr, "note file must exist on disk")
}

// parseFieldSlice handles empty, simple, and multi-value inputs. Broken
// parsing would cause --field args to silently not take effect.
func TestParseFieldSlice(t *testing.T) {
	assert.Empty(t, parseFieldSlice(nil))
	assert.Empty(t, parseFieldSlice([]string{""}), "empty string should produce no entry")
	assert.Equal(t, map[string]string{"a": "b"}, parseFieldSlice([]string{"a=b"}))
	assert.Equal(t,
		map[string]string{"a": "b", "c": "d"},
		parseFieldSlice([]string{"a=b", "c=d"}),
	)
	// Value-less key retains empty value.
	assert.Equal(t, map[string]string{"k": ""}, parseFieldSlice([]string{"k="}))
	// "=" cuts at the FIRST sign so values containing '=' survive.
	assert.Equal(t, map[string]string{"k": "v=w"}, parseFieldSlice([]string{"k=v=w"}))
}

// lint fix-links on a clean vault reports zero changes. Regression: a false
// positive here would make every "lint" run claim it fixed something.
func TestLintFixLinks_CleanVaultReportsNoChanges(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "lint", "fix-links", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			FilesScanned int `json:"files_scanned"`
			FilesChanged int `json:"files_changed"`
			LinksFixed   int `json:"links_fixed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.FilesScanned, 0)
	assert.Equal(t, 0, env.Result.FilesChanged)
	assert.Equal(t, 0, env.Result.LinksFixed)
}

// lint fix-links human output (non-JSON mode) surfaces the mode tag and the
// three counters. Downstream scripts tail this output for a human-readable
// audit trail.
func TestLintFixLinks_HumanOutputShowsModeAndCounters(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "lint", "fix-links", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Mode:", "mode line must appear")
	assert.Contains(t, text, "dry-run", "without --fix the mode is dry-run")
	assert.Contains(t, text, "Files scanned:")
	assert.Contains(t, text, "Files changed:")
	assert.Contains(t, text, "Links fixed:")
}

// --fix flag switches the mode line — the script contract is that `Mode:
// fix` is visible so operators know the action was taken.
func TestLintFixLinks_FixModeLabel(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "lint", "fix-links", "--vault", vault, "--fix")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Mode: fix")
}

// formatIndexResult: full rebuild emits "Indexed N notes (...)"; incremental
// emits the skipped/updated/added/deleted breakdown. Regression: swapping
// the two paths would confuse the user about whether everything was rebuilt.
func TestFormatIndexResult_FullRebuildMessage(t *testing.T) {
	var buf bytes.Buffer
	err := formatIndexResult(index.IndexAndEmbedResult{
		Index: &index.IndexResult{
			FullRebuild: true, Indexed: 12,
			DomainNotes: 10, UnstructuredNotes: 2, Errors: 0,
		},
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "Indexed 12 notes")
	assert.Contains(t, out, "10 domain")
	assert.Contains(t, out, "2 unstructured")
}

func TestFormatIndexResult_IncrementalMessageBreakdown(t *testing.T) {
	var buf bytes.Buffer
	err := formatIndexResult(index.IndexAndEmbedResult{
		Index: &index.IndexResult{
			FullRebuild: false, Skipped: 10, Updated: 1, Added: 2, Deleted: 0,
		},
		Embed: &index.EmbedResult{Embedded: 3, Skipped: 0, Errors: 0},
	}, &buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "10 skipped")
	assert.Contains(t, out, "1 updated")
	assert.Contains(t, out, "2 added")
	assert.Contains(t, out, "Embedded 3 notes")
}

// memory related human output includes edge_type and confidence with each
// item — those two fields are what a user uses to judge whether to follow
// the link. The test pins the format via the CLI so formatRelated stays
// honest.
func TestMemoryRelated_HumanOutputCarriesEdgeAndConfidence(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "related", "concept-alpha",
		"--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "proj-beta", "related must surface proj-beta as a neighbor")
	assert.Contains(t, text, "edge=", "human output must show the edge kind")
	assert.Contains(t, text, "confidence=", "human output must show confidence")
	assert.Contains(t, text, "related (mode:", "trailing summary must carry mode")
}

// formatSummarize includes a "NOT FOUND:" prefix for missing IDs. A script
// reading this output greps for that token; losing the prefix breaks audits.
func TestFormatSummarize_NotFoundPrefix(t *testing.T) {
	vault := buildIndexedTestVault(t)
	// Easiest: go through the CLI in human mode.
	out, _, err := runRootCmd(t, "memory", "summarize",
		"does-not-exist", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "NOT FOUND: does-not-exist")
}

// formatExperimentReport renders the table header + one row per variant.
// The column order is the script contract — flipping columns would break
// awk pipelines.
func TestFormatExperimentReport_RendersHeaderAndRows(t *testing.T) {
	report := &experiment.ReportResult{
		K: 5, SessionCount: 3, EventCount: 9, OutcomeCount: 2,
		Variants: map[string]experiment.VariantMetrics{
			"hybrid":        {HitAtK: 0.80, MRR: 0.55, EventCount: 4},
			"activation_v1": {HitAtK: 0.92, MRR: 0.71, EventCount: 5},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, formatExperimentReport(report, "activation", &buf))
	out := buf.String()
	assert.Contains(t, out, "Experiment: activation")
	assert.Contains(t, out, "Hit@5")
	assert.Contains(t, out, "MRR")
	assert.Contains(t, out, "hybrid")
	assert.Contains(t, out, "activation_v1")
	// Variants sorted alphabetically: activation_v1 < hybrid
	assert.Less(t, strings.Index(out, "activation_v1"), strings.Index(out, "hybrid"))
}

// dataviewLintText renders a human-readable summary line followed by one
// line per issue. Changing the "Checked N files" prefix would break grep
// scripts.
func TestDataviewLintText_RendersSummaryAndIssues(t *testing.T) {
	result := dataviewLintResult{
		FilesChecked: 3, Valid: 2,
		Issues: []dataviewIssue{
			{Path: "x.md", Rule: "unterminated_start", Message: "no end marker"},
		},
	}
	var buf bytes.Buffer
	// dataviewLintText takes *cobra.Command; use a bare one with Out set.
	cmd := dataviewLintCmd
	cmd.SetOut(&buf)
	require.NoError(t, dataviewLintText(cmd, result))
	out := buf.String()
	assert.Contains(t, out, "Checked 3 files: 2 valid, 1 issues")
	assert.Contains(t, out, "unterminated_start")
	assert.Contains(t, out, "no end marker")
}

// dataviewRenderText must distinguish dry-run, dry-run-with-diff, and real
// run output so users know what happened.
func TestDataviewRenderText_ModeDistinctions(t *testing.T) {
	cmd := dataviewRenderCmd
	// Real render
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	require.NoError(t, dataviewRenderText(cmd, &marker.RenderResult{
		Path: "x.md", SectionKey: "list",
	}))
	assert.Contains(t, buf.String(), "rendered x.md section list")

	// Dry run with diff
	buf.Reset()
	require.NoError(t, dataviewRenderText(cmd, &marker.RenderResult{
		Path: "x.md", SectionKey: "list", DryRun: true, Diff: "+ added\n",
	}))
	assert.Contains(t, buf.String(), "+ added")

	// Dry run without diff
	buf.Reset()
	require.NoError(t, dataviewRenderText(cmd, &marker.RenderResult{
		Path: "x.md", SectionKey: "list", DryRun: true,
	}))
	assert.Contains(t, buf.String(), "Dry run")
	assert.Contains(t, buf.String(), "no changes written")
}
