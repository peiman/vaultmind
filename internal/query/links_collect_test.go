package query_test

// links_collect_test.go — behavior-focused tests for CollectOut, CollectIn,
// CollectBoth, and the renderOut/renderIn human-output branches in links.go.
//
// These tests target the gap identified in the patch-coverage gate run (75.27%).
// CollectBoth was at 0%; CollectOut/CollectIn error paths at 75%; renderOut/renderIn
// JSON paths were already covered — these cover the complementary branches.

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CollectBoth combines outbound and inbound links for a single note into one
// result without rendering. The cmd layer uses this to emit ONE valid JSON
// envelope for --both instead of two concatenated envelopes (invalid JSON).
// This test verifies structural correctness and that out comes before in in the
// serialized form.
func TestCollectBoth_ReturnsCombinedDirections(t *testing.T) {
	db, dir := smallIndexedVault(t)
	cfg := query.LinksConfig{
		Input:     "concept-alpha",
		VaultPath: dir,
	}

	both, noteID, err := query.CollectBoth(db, cfg)
	require.NoError(t, err)
	assert.Equal(t, "concept-alpha", noteID,
		"CollectBoth must return the resolved noteID so callers can use it for logging")

	// out side: source_id is the queried note; it must have a link to proj-beta
	assert.Equal(t, "concept-alpha", both.Out.SourceID,
		"combined result's out.source_id must be the queried note")
	var foundOut bool
	for _, l := range both.Out.Links {
		if l.TargetID != nil && *l.TargetID == "proj-beta" {
			foundOut = true
		}
	}
	assert.True(t, foundOut, "out direction must include the alpha→beta outbound link")

	// in side: target_id is the queried note; beta must reference it back
	assert.Equal(t, "concept-alpha", both.In.TargetID,
		"combined result's in.target_id must be the queried note")
	var foundIn bool
	for _, l := range both.In.Links {
		if l.SourceID == "proj-beta" {
			foundIn = true
		}
	}
	assert.True(t, foundIn, "in direction must surface the beta→alpha inbound link")
}

// CollectBoth serialized to JSON must have "out" before "in" in the payload —
// the ordering is pinned by the struct field order in BothResult.
func TestCollectBoth_JSONKeyOrderIsOutBeforeIn(t *testing.T) {
	db, dir := smallIndexedVault(t)
	cfg := query.LinksConfig{Input: "concept-alpha", VaultPath: dir}

	both, _, err := query.CollectBoth(db, cfg)
	require.NoError(t, err)

	raw, err := json.Marshal(both)
	require.NoError(t, err)
	outIdx := bytes.Index(raw, []byte(`"out"`))
	inIdx := bytes.Index(raw, []byte(`"in"`))
	require.NotEqual(t, -1, outIdx, "serialized BothResult must contain \"out\" key")
	require.NotEqual(t, -1, inIdx, "serialized BothResult must contain \"in\" key")
	assert.Less(t, outIdx, inIdx, "\"out\" key must appear before \"in\" in the serialized payload")
}

// CollectBoth with an unresolvable input returns an error — callers must not
// silently get empty data.
func TestCollectBoth_UnresolvableInputErrors(t *testing.T) {
	db, dir := smallIndexedVault(t)
	cfg := query.LinksConfig{Input: "no-such-note", VaultPath: dir}

	_, _, err := query.CollectBoth(db, cfg)
	require.Error(t, err, "CollectBoth must propagate resolution errors")
}

// CollectOut returns the outbound-links payload without rendering. The caller
// (cmd layer or the CollectBoth aggregator) can then wrap it in any envelope.
func TestCollectOut_ReturnsOutboundLinks(t *testing.T) {
	db, _ := smallIndexedVault(t)

	out, err := query.CollectOut(db, "concept-alpha", "")
	require.NoError(t, err)
	assert.Equal(t, "concept-alpha", out.SourceID)

	var found bool
	for _, l := range out.Links {
		if l.TargetID != nil && *l.TargetID == "proj-beta" {
			found = true
		}
	}
	assert.True(t, found, "CollectOut must include the alpha→beta outbound link")
}

// CollectIn returns the inbound-links payload without rendering. Alpha is
// referenced by beta via both wikilink and related_ids.
func TestCollectIn_ReturnsInboundLinks(t *testing.T) {
	db, _ := smallIndexedVault(t)

	in, err := query.CollectIn(db, "concept-alpha", "")
	require.NoError(t, err)
	assert.Equal(t, "concept-alpha", in.TargetID)

	var found bool
	for _, l := range in.Links {
		if l.SourceID == "proj-beta" {
			found = true
		}
	}
	assert.True(t, found, "CollectIn must surface the beta→alpha inbound link")
}

// CollectOut/CollectIn with an edge-type filter: a non-matching filter must
// return an empty links slice, not an error — empty results are valid.
func TestCollectOut_EdgeTypeFilterDropsNonMatchingLinks(t *testing.T) {
	db, _ := smallIndexedVault(t)

	out, err := query.CollectOut(db, "concept-alpha", "no-such-edge-type")
	require.NoError(t, err)
	assert.Equal(t, "concept-alpha", out.SourceID)
	// The filter is valid; it just returns nothing. The SourceID is always set.
	var hasBeta bool
	for _, l := range out.Links {
		if l.TargetID != nil && *l.TargetID == "proj-beta" {
			hasBeta = true
		}
	}
	assert.False(t, hasBeta, "a non-matching edge-type filter must drop the alpha→beta link")
}

// RunLinks --out JSON path: the envelope must carry status "ok" and include
// the source_id. Complements the existing human-mode test.
func TestRunLinks_OutJSONEnvelopeShape(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "out", VaultPath: dir, JSONOutput: true,
	}, &buf)
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			SourceID string `json:"source_id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "concept-alpha", env.Result.SourceID)
}

// RunLinks --in JSON path: the envelope must carry status "ok" and include
// the target_id.
func TestRunLinks_InJSONEnvelopeShape(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "in", VaultPath: dir, JSONOutput: true,
	}, &buf)
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			TargetID string `json:"target_id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "concept-alpha", env.Result.TargetID)
}

// RunLinks with an unresolvable input in JSON mode emits an error envelope
// rather than returning a Go error, so callers can parse the failure code.
func TestRunLinks_UnresolvableInputJSONEmitsErrorEnvelope(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "no-such-note", Direction: "out", VaultPath: dir, JSONOutput: true,
	}, &buf)
	require.NoError(t, err, "JSON mode: error is encoded in the envelope, not returned as Go error")

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "resolution_failed", env.Errors[0].Code)
}

// renderOut human mode (non-JSON): each link is formatted with a TargetID
// pointer resolved. If TargetID is nil, TargetRaw is used. Both branches
// are exercised transitively through CollectOut + RunLinks.
func TestRunLinks_OutHumanOutputUsesTargetID(t *testing.T) {
	db, dir := smallIndexedVault(t)
	var buf bytes.Buffer
	err := query.RunLinks(db, query.LinksConfig{
		Input: "concept-alpha", Direction: "out", VaultPath: dir,
	}, &buf)
	require.NoError(t, err)
	// The human line format is: %-20s %-20s %s (target, edge_type, confidence).
	// proj-beta must appear because alpha links to it.
	assert.Contains(t, buf.String(), "proj-beta",
		"human renderOut must include the resolved target ID")
}
