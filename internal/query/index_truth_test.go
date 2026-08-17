package query_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These three findings are the same defect wearing different clothes: doctor
// declares that it checks whether the index still describes the vault, and each
// version of that check is structurally incapable of reporting a problem.
//
//   - index_status is a string literal in two constructors, never assigned again
//   - DetectContentDrift `continue`s on any read error, so a deleted file is silent
//   - duplicate_ids runs GROUP BY ... HAVING COUNT(*) > 1 against a UNIQUE column
//
// Every test below fails against the code as it stands, and each one fails
// because the check cannot fail — which is the reason to fix them together
// rather than one at a time.

// indexedVault writes notes, indexes them, and returns (vaultDir, db). The DB
// is the real one the indexer wrote, so these tests exercise what doctor sees
// in production rather than hand-inserted rows.
func indexedVault(t *testing.T, notes map[string]string) (string, *index.DB) {
	t.Helper()
	vaultDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, ".vaultmind", "config.yaml"),
		[]byte("types:\n  reference:\n    required: [title]\n"), 0o644))
	for name, body := range notes {
		full := filepath.Join(vaultDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	_, err = index.NewIndexer(vaultDir, dbPath, cfg).Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return vaultDir, db
}

func note(id, title string) string {
	return "---\nid: " + id + "\ntype: reference\ntitle: " + title + "\n---\nBody of " + title + ".\n"
}

// A note indexed and then deleted leaves a row pointing at nothing. doctor is
// the command whose job is to say "run index" — instead it reports a healthy,
// current vault, because the read error is swallowed.
func TestDoctor_ReportsAnIndexedNoteThatIsGoneFromDisk(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"kept.md": note("ref-kept", "Kept"),
		"gone.md": note("ref-gone", "Gone"),
	})
	require.NoError(t, os.Remove(filepath.Join(vaultDir, "gone.md")))

	res, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)
	assert.Equal(t, "stale", res.IndexStatus,
		"a row pointing at a file that no longer exists is not a current index")
}

// Two files claiming one id: the indexer stores the first and skips the second,
// so the table holds exactly one row and the UNIQUE-column duplicate query
// returns 0. The second file is absent from the index and from the report.
func TestDoctor_ReportsTwoFilesClaimingOneID(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"a.md": note("ref-same", "First"),
		"b.md": note("ref-same", "Second"),
	})

	res, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Issues.DuplicateIDs,
		"one id is claimed by two files on disk; the SQL check cannot see it because notes.id is UNIQUE")
	assert.Equal(t, "stale", res.IndexStatus)
}

// A note added after the last index pass is in no query result and in no
// report. total_files comes from the DB, so it does not even show up as a
// count mismatch.
func TestDoctor_ReportsAFileOnDiskWithNoIndexRow(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"indexed.md": note("ref-indexed", "Indexed"),
	})
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "new.md"),
		[]byte(note("ref-new", "New")), 0o644))

	res, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)
	assert.Equal(t, "stale", res.IndexStatus,
		"a note on disk that was never indexed means the index does not describe the vault")
}

// The false-positive guard. A check that fires on a healthy vault gets ignored,
// then disabled, and then the real signal is gone too — which is how the
// mtime-based drift detector died.
func TestDoctor_CleanVaultStaysCurrent(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"one.md":          note("ref-one", "One"),
		"nested/two.md":   note("ref-two", "Two"),
		"nested/three.md": note("ref-three", "Three"),
	})

	res, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)
	assert.Equal(t, "current", res.IndexStatus)
	assert.Equal(t, 0, res.Issues.DuplicateIDs)
	assert.Equal(t, 0, res.Issues.StaleIndex)
}

// A *.md symlink is deliberately not indexed (it could point anywhere the user
// can read). It must therefore NOT read as "a file on disk with no index row" —
// that would make every vault with a symlink permanently stale, which is the
// same false-positive death spiral. Guards the interaction with the scanner
// guard added in #106.
func TestDoctor_SymlinkIsNotReportedAsUnindexed(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"real.md": note("ref-real", "Real"),
	})
	outside := t.TempDir()
	target := filepath.Join(outside, "elsewhere.md")
	require.NoError(t, os.WriteFile(target, []byte(note("ref-outside", "Outside")), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(vaultDir, "link.md")))

	res, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)
	assert.Equal(t, "current", res.IndexStatus,
		"a skipped symlink is a deliberate exclusion, not a stale index")
}

// The count is not the finding — the path is. Every one of these categories was
// invisible before, so a number with nothing behind it would just be a
// different way of not saying which note.
func TestReconcileIndex_NamesEveryFinding(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"kept.md":  note("ref-kept", "Kept"),
		"gone.md":  note("ref-gone", "Gone"),
		"dupe1.md": note("ref-dupe", "One"),
		"dupe2.md": note("ref-dupe", "Two"),
	})
	require.NoError(t, os.Remove(filepath.Join(vaultDir, "gone.md")))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "fresh.md"),
		[]byte(note("ref-fresh", "Fresh")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "kept.md"),
		[]byte(note("ref-kept", "Kept")+"\nedited after indexing\n"), 0o644))

	truth, err := query.ReconcileIndex(db, vaultDir)
	require.NoError(t, err)

	require.Len(t, truth.Orphaned, 1)
	assert.Equal(t, "gone.md", truth.Orphaned[0].Path)
	assert.Equal(t, "ref-gone", truth.Orphaned[0].NoteID)

	assert.Equal(t, []string{"fresh.md"}, truth.Unindexed,
		"dupe2.md has no index row either, but it is reported as a duplicate — calling it "+
			"unindexed would attach the remedy `run index`, which skips it for the same reason")

	require.Len(t, truth.Duplicate, 1)
	assert.Equal(t, "ref-dupe", truth.Duplicate[0].ID)
	assert.Equal(t, []string{"dupe1.md", "dupe2.md"}, truth.Duplicate[0].Paths,
		"both claimants named — the one the indexer skipped is the one you cannot otherwise find")

	require.Len(t, truth.Drifted, 1)
	assert.Equal(t, "kept.md", truth.Drifted[0].Path)

	assert.False(t, truth.IsCurrent())
	assert.Equal(t, query.IndexStatusStale, truth.Status())
}

// `index` and `doctor` must agree about which file kept a contested id.
// Disagreement would send the operator to the wrong file.
func TestReconcileIndex_AgreesWithIndexerAboutDuplicates(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"a.md": note("ref-same", "First"),
		"b.md": note("ref-same", "Second"),
	})

	truth, err := query.ReconcileIndex(db, vaultDir)
	require.NoError(t, err)
	require.Len(t, truth.Duplicate, 1)

	// Whichever path the index actually holds must be one of the claimants
	// doctor names, and the other must be the one that is absent.
	var indexedPath string
	require.NoError(t, db.QueryRow("SELECT path FROM notes WHERE id = ?", "ref-same").Scan(&indexedPath))
	assert.Contains(t, truth.Duplicate[0].Paths, indexedPath)
	assert.Len(t, truth.Duplicate[0].Paths, 2)
}

// "Could not check" must never render as "checked, and current" — the exact
// collapse this whole reconciliation exists to undo. doctor still returns a
// report, because one unavailable section must not wedge the health hub.
func TestDoctor_UnreachableVaultIsUnknownNotCurrent(t *testing.T) {
	_, db := indexedVault(t, map[string]string{"one.md": note("ref-one", "One")})

	res, err := query.Doctor(db, filepath.Join(t.TempDir(), "does-not-exist"), nil)
	require.NoError(t, err, "a health hub must still report the sections it can")
	assert.Equal(t, query.IndexStatusUnknown, res.IndexStatus)
	assert.NotEmpty(t, res.IndexStatusReason, "say what stopped it")
	assert.NotEqual(t, query.IndexStatusCurrent, res.IndexStatus)
}

// An unreadable file is neither changed nor missing. Skipping it keeps the
// contract DetectContentDrift has always had.
func TestReconcileIndex_UnreadableFileDoesNotAbortTheRun(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"readable.md":   note("ref-readable", "Readable"),
		"unreadable.md": note("ref-unreadable", "Unreadable"),
	})
	target := filepath.Join(vaultDir, "unreadable.md")
	require.NoError(t, os.Chmod(target, 0o000))
	t.Cleanup(func() { _ = os.Chmod(target, 0o644) })

	truth, err := query.ReconcileIndex(db, vaultDir)
	require.NoError(t, err, "a per-file IO problem must not abort a health run")
	assert.Empty(t, truth.Orphaned, "unreadable is not missing")
	assert.Empty(t, truth.Drifted, "unreadable is not changed")
}

// vault status and doctor read the same reconciliation, so they cannot
// disagree about whether the index is current.
func TestVaultStatus_IndexStaleMatchesDoctor(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{"one.md": note("ref-one", "One")})
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "two.md"),
		[]byte(note("ref-two", "Two")), 0o644))

	cfg, err := vault.LoadConfig(vaultDir)
	require.NoError(t, err)
	st, err := query.VaultStatus(db, vaultDir, cfg, nil)
	require.NoError(t, err)
	doc, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)

	assert.Equal(t, doc.IndexStatus, st.IndexStatus)
	assert.True(t, st.IndexStale, "IndexStale was declared and never written")
}

// A ⚠ line on the page and "0 warnings" at the bottom is H1 inverted: the
// finding is printed and the summary denies it. Anything the report surfaces
// as an issue must be counted as one.
func TestSurfacedIssueCounts_CountsOrphanedAndUnindexed(t *testing.T) {
	errs, warns := query.SurfacedIssueCounts(query.DoctorIssues{OrphanedEntries: 2})
	assert.Equal(t, 0, errs)
	assert.Equal(t, 2, warns, "an orphaned entry prints a warning; the rollup must say so")

	errs, warns = query.SurfacedIssueCounts(query.DoctorIssues{UnindexedFiles: 3})
	assert.Equal(t, 0, errs)
	assert.Equal(t, 3, warns)

	// And together with the two that already counted, so nothing double-counts
	// or drops when they co-occur.
	errs, warns = query.SurfacedIssueCounts(query.DoctorIssues{
		StaleIndex: 1, OrphanedEntries: 2, UnindexedFiles: 3, DuplicateIDs: 4,
	})
	assert.Equal(t, 4, errs, "duplicate ids are an integrity error, not an advisory")
	assert.Equal(t, 6, warns, "1 stale + 2 orphaned + 3 unindexed")
}

// The end-to-end version: a broken vault must not produce a report whose
// bottom line says the vault is fine.
func TestDoctor_RollupMatchesTheFindingsItPrints(t *testing.T) {
	vaultDir, db := indexedVault(t, map[string]string{
		"kept.md": note("ref-kept", "Kept"),
		"gone.md": note("ref-gone", "Gone"),
	})
	require.NoError(t, os.Remove(filepath.Join(vaultDir, "gone.md")))
	require.NoError(t, os.WriteFile(filepath.Join(vaultDir, "fresh.md"),
		[]byte(note("ref-fresh", "Fresh")), 0o644))

	res, err := query.Doctor(db, vaultDir, nil)
	require.NoError(t, err)
	require.Equal(t, 1, res.Issues.OrphanedEntries)
	require.Equal(t, 1, res.Issues.UnindexedFiles)

	_, warns := query.ResultSurfacedIssueCounts(res)
	assert.Equal(t, 2, warns,
		"doctor prints two warnings here; a rollup saying 0 would deny its own output")
}
