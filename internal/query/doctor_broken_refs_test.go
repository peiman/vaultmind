package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoctor_BrokenReferences_PopulatesIssuesField verifies that when a note
// has a frontmatter explicit_relation edge pointing to a non-existent note ID
// (a broken reference), Doctor populates BrokenReferences in the returned
// DoctorIssues — and that SurfacedIssueCounts counts it as a warning.
//
// This is the regression test for the silent-under-report bug: the field was
// declared in DoctorIssues and included in SurfacedIssueCounts, but the
// query.Doctor loop never assigned to it. Real broken references reached the
// validator (ValidateResult.Issues with rule="broken_reference") but never
// reached the surfaced count.
func TestDoctor_BrokenReferences_PopulatesIssuesField(t *testing.T) {
	dir := t.TempDir()

	// Build a minimal vault with the notes schema.
	db, err := index.Open(dir + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// A source note whose frontmatter has an explicit_relation to a
	// non-existent note ID — this is the "broken reference" shape.
	_, err = db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"broken-ref-src", "broken-ref-src.md", "Broken Ref Source", "deadbeef", 0, true,
	)
	require.NoError(t, err)

	// Insert an explicit_relation edge whose dst_raw is an ID that does NOT
	// exist in the notes table — the shape countBrokenRefs detects.
	_, err = db.Exec(
		`INSERT INTO links (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"broken-ref-src", nil, "nonexistent-note-id", "explicit_relation", false, "high",
	)
	require.NoError(t, err)

	// broken_reference checking runs for every domain note regardless of type;
	// the fixture's type ("concept", with the note's title present) is chosen
	// only so the note generates NO other finding (no unknown_type, no
	// missing_required_field), leaving the broken reference as the sole issue
	// the assertions below isolate.
	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Required: []string{"title"}},
	})

	result, docErr := query.Doctor(db, dir, reg)
	require.NoError(t, docErr)

	assert.Greater(t, result.Issues.BrokenReferences, 0,
		"Doctor must populate BrokenReferences when the validator finds broken_reference issues")

	// SurfacedIssueCounts must include BrokenReferences in the warning count.
	_, warns := query.SurfacedIssueCounts(result.Issues)
	assert.Greater(t, warns, 0,
		"SurfacedIssueCounts must count BrokenReferences as warnings")
}
