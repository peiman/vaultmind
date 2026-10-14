package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func addClosesGap(t *testing.T, dir, rel, gapID string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(raw)
	require.NotContains(t, text, "closes_gap:")
	i := strings.Index(text, "\n---\n")
	require.GreaterOrEqual(t, i, 0)
	text = text[:i] + "\ncloses_gap: " + gapID + text[i:]
	require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
	rebuildVault(t, dir)
}

func proposeDraft(t *testing.T, dir string) NoteProposeEnvelope {
	t.Helper()
	proposed, errOut, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--question", "How does quartz work?",
		"--finding", "Quartz rebuilds the site when a note is saved.",
		"--json")
	require.NoError(t, err, errOut.String())
	var draft NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(proposed.Bytes(), &draft))
	require.Equal(t, "new", draft.Result.Mode)
	require.NotEmpty(t, draft.Result.Path)
	return draft
}

func TestContract_NoteAcceptClosesTheLinkedGap(t *testing.T) {
	dir := proposeVault(t)
	gapOut, gapErr, err := runRootCmd(t, "note", "gap", "How does quartz work?", "--vault", dir, "--json")
	require.NoError(t, err, gapErr.String())
	var opened NoteGapEnvelope
	require.NoError(t, json.Unmarshal(gapOut.Bytes(), &opened))
	require.NotEmpty(t, opened.Result.ID)

	proposed, propErr, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--gap", opened.Result.ID,
		"--finding", "Quartz rebuilds the site when a note is saved.",
		"--json")
	require.NoError(t, err, propErr.String())
	var draft NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(proposed.Bytes(), &draft))

	out, accErr, err := runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir, "--json")
	require.NoError(t, err, accErr.String())
	var env NoteAcceptEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note accept", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "accepted", env.Result.Status)
	assert.Equal(t, draft.Result.ID, env.Result.ID)

	raw, err := os.ReadFile(filepath.Join(dir, opened.Result.Path))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "status: closed")
	assert.Contains(t, string(raw), "closed_by: "+draft.Result.ID)
}

func TestContract_NoteAcceptWarnsWhenTheLinkedGapIsMissing(t *testing.T) {
	const gapID = "gap-missing"
	const warning = `gap "gap-missing" is not in the vault; accepted the note anyway`

	t.Run("text", func(t *testing.T) {
		dir := proposeVault(t)
		draft := proposeDraft(t, dir)
		addClosesGap(t, dir, draft.Result.Path, gapID)
		out, errOut, err := runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir)
		require.NoError(t, err, "stdout: %s\nstderr: %s", out.String(), errOut.String())
		assert.Contains(t, errOut.String(), "warning: "+warning)
		assert.Contains(t, out.String(), "accepted "+draft.Result.ID)
		raw, err := os.ReadFile(filepath.Join(dir, draft.Result.Path))
		require.NoError(t, err)
		assert.Contains(t, string(raw), "status: accepted")
	})

	t.Run("json", func(t *testing.T) {
		dir := proposeVault(t)
		draft := proposeDraft(t, dir)
		addClosesGap(t, dir, draft.Result.Path, gapID)
		out, errOut, err := runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir, "--json")
		require.NoError(t, err, errOut.String())
		var env struct {
			SchemaVersion string `json:"schema_version"`
			Command       string `json:"command"`
			Status        string `json:"status"`
			Warnings      []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Field   string `json:"field"`
			} `json:"warnings"`
			Result struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"result"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		assert.Equal(t, "v1", env.SchemaVersion)
		assert.Equal(t, "note accept", env.Command)
		assert.Equal(t, "warning", env.Status)
		assert.Equal(t, "accepted", env.Result.Status)
		require.Len(t, env.Warnings, 1)
		assert.Equal(t, "gap_missing", env.Warnings[0].Code)
		assert.Equal(t, warning, env.Warnings[0].Message)
		assert.Equal(t, gapID, env.Warnings[0].Field)
	})
}

func TestContract_NoteAcceptRejectsALinkedNoteThatIsNotAGap(t *testing.T) {
	dir := proposeVault(t)
	writeVaultNote(t, dir, "concepts/other.md", "concept-other", "Other", "accepted", "A published note.")
	rebuildVault(t, dir)
	draft := proposeDraft(t, dir)
	addClosesGap(t, dir, draft.Result.Path, "concept-other")

	out, _, err := runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir, "--json")
	require.ErrorIs(t, err, cmdutil.ErrAlreadyWritten)
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Errors        []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note accept", env.Command)
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "gap_invalid", env.Errors[0].Code)
	assert.Contains(t, env.Errors[0].Message, "not a gap")

	raw, err := os.ReadFile(filepath.Join(dir, draft.Result.Path))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "status: draft")
}
