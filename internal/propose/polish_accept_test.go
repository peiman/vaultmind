package propose

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccept_SecondAcceptOfAnAppliedPatchChangesNothing(t *testing.T) {
	v, rel, targetID := proposePatch(t, "concept", "concepts/memory.md", "concept-memory", nil)
	req, out, _, embeds := reqOf("", "")
	fm, _ := parseNote(t, v, rel)
	req.ID, _ = fm["id"].(string)
	require.NoError(t, Accept(context.Background(), v, req))

	targetAbs := filepath.Join(v.Path, "concepts", "memory.md")
	before, err := os.ReadFile(targetAbs)
	require.NoError(t, err)
	info, err := os.Stat(targetAbs)
	require.NoError(t, err)
	mtime := info.ModTime()
	embedsAtFirst := *embeds
	out.Reset()

	require.NoError(t, Accept(context.Background(), v, req))
	assert.Contains(t, out.String(), "already accepted into "+targetID+"\n")
	after, err := os.ReadFile(targetAbs)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	info, err = os.Stat(targetAbs)
	require.NoError(t, err)
	assert.True(t, mtime.Equal(info.ModTime()))
	assert.Equal(t, embedsAtFirst, *embeds, "a second accept does not re-index or embed")
}

func TestAccept_SecondAcceptJSONReportsAlready(t *testing.T) {
	v, rel, targetID := proposePatch(t, "concept", "concepts/memory.md", "concept-memory", nil)
	req, _, _, _ := reqOf("", "")
	fm, _ := parseNote(t, v, rel)
	req.ID, _ = fm["id"].(string)
	require.NoError(t, Accept(context.Background(), v, req))

	req.JSON = true
	out := &strings.Builder{}
	req.Stdout = out
	require.NoError(t, Accept(context.Background(), v, req))

	var env struct {
		Status string `json:"status"`
		Result struct {
			Mode string `json:"mode"`
			Path string `json:"path"`
			ID   string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.String()), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "already", env.Result.Mode)
	assert.Equal(t, "concepts/memory.md", env.Result.Path)
	assert.Equal(t, targetID, env.Result.ID)
}

func TestAccept_FindsTheMarkerInTheFileWhenRawBodyIsEmpty(t *testing.T) {
	v, rel, targetID := proposePatch(t, "concept", "concepts/memory.md", "concept-memory", nil)
	req, out, _, _ := reqOf("", "")
	fm, _ := parseNote(t, v, rel)
	req.ID, _ = fm["id"].(string)
	require.NoError(t, Accept(context.Background(), v, req))
	_, err := v.DB.Exec(`UPDATE notes SET body_raw = NULL WHERE id = ?`, targetID)
	require.NoError(t, err)

	targetAbs := filepath.Join(v.Path, "concepts", "memory.md")
	before, err := os.ReadFile(targetAbs)
	require.NoError(t, err)
	out.Reset()
	require.NoError(t, Accept(context.Background(), v, req))
	assert.Contains(t, out.String(), "already accepted into "+targetID)
	after, err := os.ReadFile(targetAbs)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestAccept_MarkerOnlyInTheHeadOfALargeFileIsNotAHit(t *testing.T) {
	v := testVault(t)
	const proposalID = "proposal-only-in-the-head"
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "short")
	_, err := v.DB.Exec(`UPDATE notes SET body_raw = NULL WHERE id = ?`, "concept-memory")
	require.NoError(t, err)
	abs := filepath.Join(v.Path, "concepts", "memory.md")
	head, err := os.ReadFile(abs)
	require.NoError(t, err)
	var b strings.Builder
	b.Write(head)
	b.WriteString("\n" + acceptedMarker(proposalID) + "\n")
	b.WriteString(strings.Repeat("x", int(acceptedScanMaxBytes)))
	require.NoError(t, os.WriteFile(abs, []byte(b.String()), 0o644))

	req, _, _, _ := reqOf("", "")
	req.ID = proposalID
	err = Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the vault")
}

func TestAccept_MarkerAtTheEndOfALargeFileIsFound(t *testing.T) {
	v := testVault(t)
	const proposalID = "proposal-at-the-end"
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "short")
	_, err := v.DB.Exec(`UPDATE notes SET body_raw = NULL WHERE id = ?`, "concept-memory")
	require.NoError(t, err)
	abs := filepath.Join(v.Path, "concepts", "memory.md")
	head, err := os.ReadFile(abs)
	require.NoError(t, err)
	var b strings.Builder
	b.Write(head)
	b.WriteString(strings.Repeat("y", int(acceptedScanMaxBytes)))
	b.WriteString("\n" + acceptedMarker(proposalID) + "\n")
	raw := []byte(b.String())
	require.NoError(t, os.WriteFile(abs, raw, 0o644))

	req, out, _, embeds := reqOf("", "")
	req.ID = proposalID
	require.NoError(t, Accept(context.Background(), v, req))
	assert.Contains(t, out.String(), "already accepted into concept-memory\n")
	assert.Equal(t, 0, *embeds)
	got, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, raw, got)
}
