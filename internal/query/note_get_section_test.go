package query_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `note get <note>#<anchor>` opens one part of a long note — the id the hit
// line shows. Opening the whole page would bury the answer again.
func TestNoteGet_ASectionIDOpensThatSection(t *testing.T) {
	db, ids := sectionedDB(t)

	var buf bytes.Buffer
	out, err := query.RunNoteGet(db, query.NoteGetConfig{Input: ids[1]}, &buf)
	require.NoError(t, err)
	assert.Equal(t, "ref-long", out.NoteID, "the access is the note's")
	assert.True(t, out.BodyDelivered)
	text := buf.String()
	assert.Contains(t, text, ids[1]+" (reference) — Long Guide › Configure")
	assert.Contains(t, text, "\nConfigure ")
	assert.NotContains(t, text, "Install ", "only that section, not the note")

	buf.Reset()
	_, err = query.RunNoteGet(db, query.NoteGetConfig{Input: ids[1], JSONOutput: true}, &buf)
	require.NoError(t, err)
	var env struct {
		Result struct {
			ID          string `json:"id"`
			Section     string `json:"section"`
			HeadingPath string `json:"heading_path"`
			Body        string `json:"body"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "ref-long", env.Result.ID)
	assert.Equal(t, ids[1], env.Result.Section)
	assert.Equal(t, "Configure", env.Result.HeadingPath)
	assert.True(t, strings.HasPrefix(env.Result.Body, "Configure"))
}

// The plain note id still opens the whole note, as before.
func TestNoteGet_TheNoteIDStillOpensTheWholeNote(t *testing.T) {
	db, _ := sectionedDB(t)
	var buf bytes.Buffer
	_, err := query.RunNoteGet(db, query.NoteGetConfig{Input: "ref-long"}, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Install ")
	assert.Contains(t, buf.String(), "Configure ")
}
