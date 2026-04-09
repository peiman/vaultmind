package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatcher_NoneVariant(t *testing.T) {
	d := experiment.NewDispatcher()

	notes := []experiment.ScoredNote{
		{NoteID: "note-1", Score: 0.9},
		{NoteID: "note-2", Score: 0.5},
	}

	result, err := d.Score("none", notes)
	require.NoError(t, err)
	assert.Equal(t, notes, result, "none scorer must return input unchanged")
}

func TestDispatcher_RegisterAndScore(t *testing.T) {
	d := experiment.NewDispatcher()

	// Register a "reverse" scorer that reverses the slice.
	d.Register("reverse", experiment.ScorerFunc(func(notes []experiment.ScoredNote) []experiment.ScoredNote {
		out := make([]experiment.ScoredNote, len(notes))
		for i, n := range notes {
			out[len(notes)-1-i] = n
		}
		return out
	}))

	notes := []experiment.ScoredNote{
		{NoteID: "note-a", Score: 0.9},
		{NoteID: "note-b", Score: 0.5},
		{NoteID: "note-c", Score: 0.1},
	}

	result, err := d.Score("reverse", notes)
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "note-c", result[0].NoteID)
	assert.Equal(t, "note-b", result[1].NoteID)
	assert.Equal(t, "note-a", result[2].NoteID)
}

func TestDispatcher_UnknownVariant(t *testing.T) {
	d := experiment.NewDispatcher()

	_, err := d.Score("nonexistent", []experiment.ScoredNote{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown variant")
}

func TestDispatcher_RunAll(t *testing.T) {
	d := experiment.NewDispatcher()

	notes := []experiment.ScoredNote{
		{NoteID: "note-x", Score: 0.7},
	}

	results, err := d.RunAll([]string{"none"}, notes)
	require.NoError(t, err)
	require.Contains(t, results, "none")
	assert.Equal(t, notes, results["none"])
}
