package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// init's two sample notes are tagged example. After they are indexed, doctor
// names them so they do not sit in the map unnoticed.
func TestDoctor_NamesInitExampleNotes(t *testing.T) {
	isolateMeshEnv(t)
	vault := filepath.Join(t.TempDir(), "kb")
	_, _, err := runRootCmd(t, "init", vault)
	require.NoError(t, err)
	_, _, err = runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "2 example notes: delete or keep")
	assert.Contains(t, out.String(), "decision-example")
	assert.Contains(t, out.String(), "concept-example")

	raw, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			ExampleNotes struct {
				Count int `json:"count"`
				Notes []struct {
					ID   string `json:"id"`
					Path string `json:"path"`
				} `json:"notes"`
			} `json:"example_notes"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw.Bytes(), &env), raw.String())
	assert.Equal(t, 2, env.Result.ExampleNotes.Count)
	require.Len(t, env.Result.ExampleNotes.Notes, 2)
	ids := []string{env.Result.ExampleNotes.Notes[0].ID, env.Result.ExampleNotes.Notes[1].ID}
	assert.ElementsMatch(t, []string{"decision-example", "concept-example"}, ids)
	// Removing the examples and indexing removes the reminder in both modes.
	for _, note := range env.Result.ExampleNotes.Notes {
		require.NoError(t, os.Remove(filepath.Join(vault, note.Path)))
	}
	_, _, err = runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)
	out, _, err = runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "example notes: delete or keep")
	raw, _, err = runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.NotContains(t, raw.String(), `"example_notes"`)

}

func TestWriteExampleNotes_SilentWhenThereAreNone(t *testing.T) {
	var b strings.Builder
	require.NoError(t, writeExampleNotes(&b, nil))
	require.NoError(t, writeExampleNotes(&b, &query.DoctorExampleNotes{}))
	assert.Empty(t, b.String())
}

func TestWriteExampleNotes_Singular(t *testing.T) {
	var b strings.Builder
	require.NoError(t, writeExampleNotes(&b, &query.DoctorExampleNotes{
		Count: 1,
		Notes: []query.DoctorExampleNote{{ID: "decision-example", Path: "decisions/decision-example.md"}},
	}))
	assert.Contains(t, b.String(), "1 example note: delete or keep")
	assert.Contains(t, b.String(), "decision-example")
}
