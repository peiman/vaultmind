package index_test

import (
	"sort"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unitVault stores a short note and a long note, both with note-level
// vectors, and returns the long note's section ids.
func unitVault(t *testing.T) (*index.DB, []string) {
	t.Helper()
	db := openTestDB(t)
	short := buildTestRecord("concept-short", "concepts/short.md")
	require.NoError(t, index.StoreNote(db, short))
	long := longRecord("ref-long", "refs/long.md", "Alpha", "Beta")
	require.NoError(t, index.StoreNote(db, long))
	for _, id := range []string{"concept-short", "ref-long"} {
		require.NoError(t, index.StoreEmbedding(db, id, []float32{1, 0}))
		require.NoError(t, index.StoreSparseEmbedding(db, id, map[int32]float32{1: 1}))
		require.NoError(t, index.StoreColBERTEmbedding(db, id, [][]float32{{1, 0}}))
	}
	var ids []string
	for _, r := range sectionsOf(t, db, "ref-long") {
		ids = append(ids, r.ID)
	}
	require.NotEmpty(t, ids)
	return db, ids
}

func embedSections(t *testing.T, db *index.DB, dense, sparse bool) {
	t.Helper()
	if dense {
		_, err := db.Exec(`UPDATE sections SET embedding = ?`, index.EncodeEmbedding([]float32{0, 1}))
		require.NoError(t, err)
	}
	if sparse {
		_, err := db.Exec(`UPDATE sections SET sparse_embedding = ?`, index.EncodeSparseEmbedding(map[int32]float32{2: 1}))
		require.NoError(t, err)
	}
}

func unitIDs(units []index.NoteEmbedding) []string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = u.UnitID()
	}
	sort.Strings(out)
	return out
}

// Until its sections are embedded, a long note is searched by its own
// (truncated) vector — it never drops out of search mid-upgrade. Once they
// are, the note is its sections: the truncated vector is not a unit.
func TestLoadUnitEmbeddings_ALongNoteIsItsSectionsOnceEmbedded(t *testing.T) {
	db, sectionIDs := unitVault(t)

	units, err := index.LoadUnitEmbeddings(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"concept-short", "ref-long"}, unitIDs(units), "sections not embedded yet")

	embedSections(t, db, true, false)
	units, err = index.LoadUnitEmbeddings(db)
	require.NoError(t, err)
	want := append([]string{"concept-short"}, sectionIDs...)
	sort.Strings(want)
	assert.Equal(t, want, unitIDs(units))
	for _, u := range units {
		if u.SectionID != "" {
			assert.Equal(t, "ref-long", u.NoteID, "a section carries its note")
			assert.Equal(t, "Test Note", u.Title, "and the note's title")
			assert.NotEmpty(t, u.BodyText, "and its own text")
		}
	}
}

// Sections left stale by an older binary (note_hash differs) are not units;
// the note's own vector serves until the sections are cut again.
func TestLoadUnitEmbeddings_StaleSectionsAreNotUnits(t *testing.T) {
	db, _ := unitVault(t)
	embedSections(t, db, true, true)
	_, err := db.Exec(`UPDATE notes SET hash = 'edited-elsewhere' WHERE id = 'ref-long'`)
	require.NoError(t, err)
	units, err := index.LoadUnitEmbeddings(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"concept-short", "ref-long"}, unitIDs(units))
}

// The sparse lane follows the same rule, on sparse vectors.
func TestLoadUnitSparseEmbeddings_FollowsTheSameRule(t *testing.T) {
	db, sectionIDs := unitVault(t)
	embedSections(t, db, true, false) // dense only: sparse still the note's
	sp, err := index.LoadUnitSparseEmbeddings(db)
	require.NoError(t, err)
	var ids []string
	for _, u := range sp {
		ids = append(ids, u.UnitID())
	}
	sort.Strings(ids)
	assert.Equal(t, []string{"concept-short", "ref-long"}, ids)

	embedSections(t, db, false, true)
	sp, err = index.LoadUnitSparseEmbeddings(db)
	require.NoError(t, err)
	ids = nil
	for _, u := range sp {
		ids = append(ids, u.UnitID())
	}
	sort.Strings(ids)
	want := append([]string{"concept-short"}, sectionIDs...)
	sort.Strings(want)
	assert.Equal(t, want, ids)
}

// Sections have no ColBERT, and a long note whose sections are embedded is
// searched by them: its truncated ColBERT is left out.
func TestColBERTLoaders_LeaveOutANoteSearchedBySections(t *testing.T) {
	db, _ := unitVault(t)
	embedSections(t, db, true, true)
	all, err := index.LoadAllColBERTEmbeddings(db, 2)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "concept-short", all[0].NoteID)
	some, err := index.LoadColBERTEmbeddingsFor(db, 2, []string{"concept-short", "ref-long"})
	require.NoError(t, err)
	require.Len(t, some, 1)
	assert.Equal(t, "concept-short", some[0].NoteID)
}

// The vectors that stand for one note: its sections once embedded, else its
// own — what a relevance check scores a hit against.
func TestLoadNoteUnitEmbeddings(t *testing.T) {
	db, sectionIDs := unitVault(t)
	vecs, err := index.LoadNoteUnitEmbeddings(db, "ref-long")
	require.NoError(t, err)
	assert.Len(t, vecs, 1, "the note's own vector before its sections are embedded")

	embedSections(t, db, true, false)
	vecs, err = index.LoadNoteUnitEmbeddings(db, "ref-long")
	require.NoError(t, err)
	assert.Len(t, vecs, len(sectionIDs))

	vecs, err = index.LoadNoteUnitEmbeddings(db, "concept-short")
	require.NoError(t, err)
	assert.Len(t, vecs, 1)
}
