package cmd

// memory_coverage_test.go — behavior tests targeting uncovered branches in
// memory_links.go (runLinksBoth human path + CollectBoth error path),
// memory_neighbors_helpers.go (formatRecall MaxNodesReached suffix),
// memory_pack.go (error + JSON error paths),
// memory_pack_helpers.go (formatContextPack Truncated=true),
// and doctor_heal.go (zero-change human output without stale-index warning).
//
// All tests assert on outputs or error messages — no vacuous line-hit tests.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bytes"
)

// ---- memory links human output for --both --------------------------------

// memory links --both (default) without --json renders two headed sections:
// "outbound:" then "inbound:". Each link appears under the correct section.
// This exercises the runLinksBoth human-output path (the JSON path is covered
// by TestMemoryLinks_DefaultBothShowsBothDirections).
func TestMemoryLinksBoth_HumanOutputShowsTwoSections(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--vault", vault)
	require.NoError(t, err)

	text := out.String()
	outIdx := strings.Index(text, "outbound:")
	inIdx := strings.Index(text, "inbound:")
	require.NotEqual(t, -1, outIdx, "human --both output must include an 'outbound:' header")
	require.NotEqual(t, -1, inIdx, "human --both output must include an 'inbound:' header")
	assert.Less(t, outIdx, inIdx, "outbound section must appear before inbound section")
}

// memory links --both human output must list links under the outbound section.
// proj-beta references alpha (back-link) AND alpha references proj-beta
// (forward link), so both sections must contain proj-beta.
func TestMemoryLinksBoth_HumanOutputContainsLinks(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--vault", vault)
	require.NoError(t, err)

	text := out.String()
	// At least one of the sections must mention proj-beta.
	assert.Contains(t, text, "proj-beta",
		"human --both output must include proj-beta in at least one direction")
}

// memory links --both with an unresolvable ID in human mode must return a
// Go error (no JSON) that is not empty.
func TestMemoryLinksBoth_HumanOutputUnresolvableErrors(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "links", "no-such-id",
		"--vault", vault)
	require.Error(t, err,
		"human mode with unresolvable ID must return a Go error")
}

// memory links --both --json with an unresolvable ID must write a JSON error
// envelope to stdout and return nil (so callers can parse the failure code).
func TestMemoryLinksBoth_JSONUnresolvableWritesErrorEnvelope(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "no-such-id",
		"--both", "--vault", vault, "--json")
	require.NoError(t, err,
		"JSON mode must encode errors in the envelope, not as a Go error")

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.True(t, json.Valid(out.Bytes()), "error output must be valid JSON")
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status,
		"envelope status must be 'error' for unresolvable input")
}

// ---- memory links --out / --in human output --------------------------------

// memory links --out human output: each row has the format
// %-20s %-20s %s (target_id/raw, edge_type, confidence). No direction header
// is printed for single-direction commands (unlike --both).
func TestMemoryLinksOut_HumanOutputNoSectionHeader(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--out", "--vault", vault)
	require.NoError(t, err)

	text := out.String()
	assert.NotContains(t, text, "outbound:", "--out human output must NOT print a section header")
	assert.NotContains(t, text, "inbound:", "--out human output must NOT print an inbound header")
	assert.Contains(t, text, "proj-beta", "--out output must list the outbound link target")
}

// memory links --in human output: similar contract to --out, no section header.
func TestMemoryLinksIn_HumanOutputNoSectionHeader(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "links", "concept-alpha",
		"--in", "--vault", vault)
	require.NoError(t, err)

	text := out.String()
	assert.NotContains(t, text, "outbound:", "--in human output must NOT print a section header")
	assert.NotContains(t, text, "inbound:", "--in human output must NOT print an inbound header")
	assert.Contains(t, text, "proj-beta", "--in output must list proj-beta as a backlink source")
}

// ---- memory neighbors human output + formatRecall branches ----------------

// memory neighbors human output (no --json) calls formatRecall. When the
// traversal fits within the node budget (MaxNodesReached=false), there must be
// no "(max reached)" suffix on the count line. This pins the "not reached"
// branch of formatRecall.
func TestMemoryNeighbors_HumanOutputNormalCountLine(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "neighbors", "concept-alpha",
		"--vault", vault, "--depth", "1", "--max-nodes", "50")
	require.NoError(t, err)

	text := out.String()
	assert.Contains(t, text, "nodes", "human output must end with a 'N nodes' line")
	assert.NotContains(t, text, "(max reached)",
		"no max-reached suffix when the node budget was not hit")
}

// formatRecall MaxNodesReached branch: when max-nodes is set to 1 on a vault
// with at least 2 reachable notes, the traversal must truncate and append
// the "(max reached)" suffix to the count line.
func TestFormatRecall_MaxNodesReachedSuffix(t *testing.T) {
	// Build the result directly and pass it to formatRecall so the test is
	// decoupled from vault I/O while still verifying real behavior.
	result := &memory.RecallResult{
		Nodes: []memory.RecallNode{
			{ID: "concept-alpha", Type: "concept", Title: "Alpha", Distance: 0},
			{ID: "proj-beta", Type: "project", Title: "Beta", Distance: 1},
		},
		MaxNodesReached: true,
	}

	var buf bytes.Buffer
	err := formatRecall(result, &buf)
	require.NoError(t, err)

	text := buf.String()
	assert.Contains(t, text, "2 nodes", "count must reflect the two nodes in the result")
	assert.Contains(t, text, "(max reached)",
		"MaxNodesReached=true must append the '(max reached)' suffix to the count line")
}

// formatRecall renders the target node at depth 0 with its ID, type, and title.
// Neighbor nodes at depth≥1 are indented with "→". This pins the two render
// branches without needing a live vault.
func TestFormatRecall_RendersDepthZeroAndNeighbors(t *testing.T) {
	result := &memory.RecallResult{
		Nodes: []memory.RecallNode{
			{ID: "concept-alpha", Type: "concept", Title: "Alpha", Distance: 0},
			{ID: "proj-beta", Type: "project", Title: "Beta", Distance: 1},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, formatRecall(result, &buf))
	text := buf.String()

	// Depth-0 line: no "→" prefix, contains (depth 0) marker.
	assert.Contains(t, text, "concept-alpha", "depth-0 node must include its ID")
	assert.Contains(t, text, "(depth 0)", "depth-0 line must be tagged with '(depth 0)'")

	// Depth-1 line: "→" prefix and depth label.
	assert.Contains(t, text, "→", "neighbor at depth≥1 must use '→' prefix")
	assert.Contains(t, text, "proj-beta", "neighbor ID must appear")
	assert.Contains(t, text, "depth 1", "neighbor must carry its distance label")
}

// ---- memory pack branches --------------------------------------------------

// memory pack --json error path: when the note cannot be resolved, the JSON
// error envelope is written to stdout (not stderr) and the command returns nil.
func TestMemoryPack_JSONErrorEnvelopeOnUnresolvableID(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "memory", "pack", "no-such-note",
		"--vault", vault, "--budget", "4000", "--max-items", "8", "--json")
	require.NoError(t, err,
		"JSON mode must encode errors in the envelope, not return a Go error")

	require.True(t, json.Valid(out.Bytes()),
		"error envelope must be valid JSON")
	var env struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status,
		"unresolvable note must yield an error-status envelope")
}

// memory pack human error path: when the note cannot be resolved and --json is
// not set, a Go error is returned (not written to stdout).
func TestMemoryPack_HumanErrorOnUnresolvableID(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "memory", "pack", "no-such-note",
		"--vault", vault, "--budget", "4000", "--max-items", "8")
	require.Error(t, err,
		"human mode must return a Go error when the note cannot be resolved")
}

// formatContextPack with Truncated=true appends " (truncated)" to the token
// line. Without this, operators cannot tell from the output that some context
// was dropped to fit the budget.
func TestFormatContextPack_TruncatedFlag(t *testing.T) {
	result := &memory.ContextPackResult{
		TargetID:     "concept-alpha",
		BudgetTokens: 100,
		UsedTokens:   95,
		Truncated:    true,
	}

	var buf bytes.Buffer
	err := formatContextPack(result, &buf)
	require.NoError(t, err)

	text := buf.String()
	assert.Contains(t, text, "tokens: 95 / 100", "token line must show used/budget")
	assert.Contains(t, text, "(truncated)",
		"Truncated=true must append '(truncated)' to the token line")
}

// formatContextPack with Truncated=false must NOT append the truncated marker.
func TestFormatContextPack_NotTruncated(t *testing.T) {
	result := &memory.ContextPackResult{
		TargetID:     "concept-alpha",
		BudgetTokens: 4000,
		UsedTokens:   200,
		Truncated:    false,
	}

	var buf bytes.Buffer
	require.NoError(t, formatContextPack(result, &buf))
	assert.NotContains(t, buf.String(), "(truncated)",
		"Truncated=false must not append the truncated marker")
	assert.Contains(t, buf.String(), "tokens: 200 / 4000")
}

// formatContextPack with a non-nil Target prints the target ID on the first line.
func TestFormatContextPack_WithTarget(t *testing.T) {
	result := &memory.ContextPackResult{
		TargetID:     "concept-alpha",
		BudgetTokens: 4000,
		UsedTokens:   50,
		Target: &memory.ContextPackTarget{
			ID: "concept-alpha",
		},
		Context: []memory.ContextItem{{ID: "x"}},
	}

	var buf bytes.Buffer
	require.NoError(t, formatContextPack(result, &buf))
	text := buf.String()
	assert.Contains(t, text, "target: concept-alpha",
		"formatContextPack must print the target line when Target is non-nil")
	assert.Contains(t, text, "1 context items",
		"formatContextPack must count context items")
}

// ---- doctor heal stale-index: zero-change human output ---------------------

// doctor heal on a vault that has no fixable links (zero changes) must NOT
// warn about a stale index in human output — the index is still valid when
// nothing was written. This pins the "zero changes" branch of runWikilinkFix
// human output.
func TestDoctorHeal_ZeroChangesHumanOutputNoStaleWarning(t *testing.T) {
	vault := buildCleanIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault)
	require.NoError(t, err)

	text := out.String()
	assert.Contains(t, text, "Files changed: 0",
		"precondition: the vault has no fixable links, so 0 files are changed")
	assert.NotContains(t, text, "Index is now stale",
		"zero-change heal must NOT warn about a stale index")
}
