package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A short accepted patch must not hand VaultMind's bookkeeping comment to an
// agent. An author comment on the same note still has to arrive. note get is
// the raw read and is allowed to show the marker.
func TestDelivery_HidesVaultMindMarkerKeepsAuthorComment(t *testing.T) {
	dir := proposeVault(t)
	const (
		targetID = "concept-memory"
		marker   = "vaultmind:accepted"
		author   = "<!-- todo -->"
		token    = "zzquartzkeep"
	)
	writeVaultNote(t, dir, "concepts/memory.md", targetID, "Memory", "accepted",
		author+"\n"+token+" one timeline.\n")
	proposal := "---\n" +
		"id: proposal-quartz\n" +
		"type: concept\n" +
		"title: How does quartz work?\n" +
		"status: draft\n" +
		"question: How does quartz work?\n" +
		"patches: " + targetID + "\n" +
		"created: 2026-04-03\n" +
		"---\n\n" +
		"Quartz keeps one timeline.\n"
	rel := filepath.Join(dir, "concepts", "proposal-quartz.md")
	require.NoError(t, os.WriteFile(rel, []byte(proposal), 0o644))
	rebuildVault(t, dir)

	_, _, err := runRootCmd(t, "note", "accept", "proposal-quartz", "--vault", dir)
	require.NoError(t, err)

	ask, _, err := runRootCmd(t, "ask", token, "--vault", dir)
	require.NoError(t, err)
	preview, _, err := runRootCmd(t, "ask", token, "--vault", dir, "--preview")
	require.NoError(t, err)
	read, _, err := runRootCmd(t, "ask", token, "--vault", dir, "--read", "1")
	require.NoError(t, err)
	pack, _, err := runRootCmd(t, "memory", "pack", targetID, "--vault", dir, "--json")
	require.NoError(t, err)
	var packed struct {
		Result struct {
			Target struct {
				Body string `json:"body"`
			} `json:"target"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(pack.Bytes(), &packed))
	packBody := packed.Result.Target.Body
	got, _, err := runRootCmd(t, "note", "get", targetID, "--vault", dir)
	require.NoError(t, err)

	for name, out := range map[string]string{
		"ask":      ask.String(),
		"preview":  preview.String(),
		"read":     read.String(),
		"pack":     packBody,
		"note get": got.String(),
	} {
		assert.Contains(t, out, author, name)
		assert.Contains(t, out, "Quartz keeps one timeline.", name)
	}
	for name, out := range map[string]string{
		"ask":     ask.String(),
		"preview": preview.String(),
		"read":    read.String(),
		"pack":    packBody,
	} {
		assert.NotContains(t, out, marker, name)
	}
	assert.Contains(t, got.String(), marker, "note get is a raw read and keeps the marker")
}
