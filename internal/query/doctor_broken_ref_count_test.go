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

// "Broken references: N" counted the NOTES carrying broken references, not the
// references. Measured live: a vault with 3 broken refs across 2 notes reported
// 2. A true number under the wrong label, with nothing in the output to let a
// reader notice the gap.
//
// The existing regression test asserts `BrokenReferences > 0`, which is true
// under both the counting-notes and counting-references implementations. That is
// exactly why this survived — the assertion could not tell them apart. The
// fixture below is built so the two answers DIFFER (3 refs, 2 notes), because a
// fixture where they coincide tests nothing.
func TestDoctor_CountsReferencesNotNotes(t *testing.T) {
	dir := t.TempDir()
	db, err := index.Open(dir + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	addNote := func(id string) {
		t.Helper()
		_, e := db.Exec(
			"INSERT INTO notes (id, path, title, type, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?, ?)",
			id, id+".md", "Title "+id, "concept", "hash-"+id, 0, true)
		require.NoError(t, e)
	}
	addBrokenRef := func(src, dst string) {
		t.Helper()
		_, e := db.Exec(
			`INSERT INTO links (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			src, nil, dst, "explicit_relation", false, "high")
		require.NoError(t, e)
	}

	// 2 notes, 3 broken references. One note carries two of them — the shape
	// that makes counting-notes and counting-references give different answers.
	addNote("note-a")
	addNote("note-b")
	addBrokenRef("note-a", "missing-one")
	addBrokenRef("note-a", "missing-two")
	addBrokenRef("note-b", "missing-one") // same target, different source

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Required: []string{"title"}},
	})

	result, docErr := query.Doctor(db, dir, reg)
	require.NoError(t, docErr)

	assert.Equal(t, 3, result.Issues.BrokenReferences,
		"3 references are broken across 2 notes; reporting 2 counts notes under a label that says references")
}

// The ids must reach the caller, not just their number. `frontmatter validate`
// printed "N frontmatter references do not resolve" — the part the reader
// already knows, without the part they need. The query resolved every id to
// decide it was broken, then aggregated them away.
//
// Fixing 8 of these by hand across two vaults turned into five different
// problems with five different remedies: one missing note referenced four times,
// an id missing its `source-` prefix, two pointing into another vault, two never
// written, and an auto-memory filename pasted into a vault's related_ids. A count
// distinguishes none of that; the ids distinguish all of it.
func TestValidate_NamesTheBrokenReferences(t *testing.T) {
	dir := t.TempDir()
	db, err := index.Open(dir + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(
		"INSERT INTO notes (id, path, title, type, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"src", "src.md", "Source", "concept", "h", 0, true)
	require.NoError(t, err)
	for _, dst := range []string{"zeta-missing", "alpha-missing"} {
		_, err = db.Exec(
			`INSERT INTO links (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			"src", nil, dst, "explicit_relation", false, "high")
		require.NoError(t, err)
	}

	reg := schema.NewRegistry(map[string]vault.TypeDef{
		"concept": {Required: []string{"title"}},
	})

	res, err := query.Validate(db, reg)
	require.NoError(t, err)

	var found *query.ValidateIssue
	for i := range res.Issues {
		if res.Issues[i].Rule == query.RuleBrokenReference {
			found = &res.Issues[i]
			break
		}
	}
	require.NotNil(t, found, "precondition: the fixture produces a broken_reference issue")

	// Sorted, so the message is stable across runs and diffable between them.
	assert.Equal(t, []string{"alpha-missing", "zeta-missing"}, found.BrokenRefs,
		"the ids must be carried, in a stable order")
	assert.Contains(t, found.Message, "alpha-missing",
		"and named in the human message — that is the surface a person reads")
	assert.Contains(t, found.Message, "zeta-missing")
}
