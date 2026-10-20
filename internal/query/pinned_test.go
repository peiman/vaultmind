package query_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A pinned id the store cannot load fails doctor. The section is measured
// before reconciliation, so a broken read must not be reported as a healthy
// vault that simply has no pins.
func TestDoctor_PinnedLoadErrorNamesTheNote(t *testing.T) {
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`INSERT INTO notes (id, path, title, type, body_text, hash, mtime) VALUES ('note-a', 'a.md', 'Alpha', 'concept', 'body', 'h', 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES ('note-a', 'pinned', 'true')`)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE aliases`)
	require.NoError(t, err)

	_, err = query.Doctor(db, dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pinned notes")
	assert.Contains(t, err.Error(), "note-a")
}

func TestResultSurfacedIssueCounts_CountsPinnedOverBudget(t *testing.T) {
	_, warns := query.ResultSurfacedIssueCounts(&query.DoctorResult{
		Pinned: &query.DoctorPinned{OverBudget: true},
	})
	assert.Equal(t, 1, warns, "the pinned warning is printed; it must be counted")

	_, under := query.ResultSurfacedIssueCounts(&query.DoctorResult{
		Pinned: &query.DoctorPinned{},
	})
	assert.Equal(t, 0, under, "a pinned set inside the budget adds no warning")
}
