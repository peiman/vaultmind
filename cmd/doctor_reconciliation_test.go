package cmd

import (
	"bytes"
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each of these categories was invisible before — two were checks that could
// not fail, the third was never checked — so the report names PATHS. A count
// with nothing behind it is the shape H1 was about.
func TestWriteIndexReconciliation_NamesPathsNotJustCounts(t *testing.T) {
	issues := &query.DoctorIssues{
		OrphanedEntries: 1,
		OrphanedEntryDetails: []query.OrphanedEntry{
			{NoteID: "ref-gone", Path: "arcs/gone.md"},
		},
		UnindexedFiles:       1,
		UnindexedFileDetails: []string{"inbox/fresh.md"},
		DuplicateIDDetails: []query.DuplicateOnDisk{
			{ID: "ref-dupe", Paths: []string{"a.md", "b.md"}},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeIndexReconciliation(&buf, issues, false))
	out := buf.String()

	assert.Contains(t, out, "arcs/gone.md")
	assert.Contains(t, out, "ref-gone")
	assert.Contains(t, out, "inbox/fresh.md")
	assert.Contains(t, out, "ref-dupe")
	assert.Contains(t, out, "a.md")
	assert.Contains(t, out, "b.md")
	assert.Contains(t, out, "cannot be opened", "say what an orphan means, not just that it exists")
	assert.Contains(t, out, "in no query result", "same for an unindexed note")
}

// --summary suppresses the per-item lines but must keep the counts: the whole
// point of the flag is a shorter report, not a quieter one.
func TestWriteIndexReconciliation_SummarySuppressesDetailsNotCounts(t *testing.T) {
	issues := &query.DoctorIssues{
		OrphanedEntries:      1,
		OrphanedEntryDetails: []query.OrphanedEntry{{NoteID: "ref-gone", Path: "arcs/gone.md"}},
	}

	var buf bytes.Buffer
	require.NoError(t, writeIndexReconciliation(&buf, issues, true))
	out := buf.String()

	assert.Contains(t, out, "1 note(s) indexed but gone from disk")
	assert.NotContains(t, out, "arcs/gone.md")
}

// A healthy vault prints nothing here. A section that always emits something
// trains the reader to skip the whole report.
func TestWriteIndexReconciliation_SilentWhenNothingIsWrong(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeIndexReconciliation(&buf, &query.DoctorIssues{}, false))
	assert.Empty(t, buf.String())
}

// Write errors propagate rather than leaving a half-printed warning that reads
// as a complete one.
func TestWriteIndexReconciliation_PropagatesWriteErrors(t *testing.T) {
	issues := &query.DoctorIssues{
		OrphanedEntries:      1,
		OrphanedEntryDetails: []query.OrphanedEntry{{NoteID: "ref-gone", Path: "arcs/gone.md"}},
		UnindexedFiles:       1,
		UnindexedFileDetails: []string{"inbox/fresh.md"},
		DuplicateIDDetails:   []query.DuplicateOnDisk{{ID: "d", Paths: []string{"a.md"}}},
	}
	for ok := 0; ok < 5; ok++ {
		require.Error(t, writeIndexReconciliation(&failAfterNWriter{ok: ok}, issues, false),
			"write failure at position %d must propagate", ok)
	}
}
