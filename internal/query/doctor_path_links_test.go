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

// Obsidian resolves a link by its vault-relative path as well as by its
// filename, and import writes path links between a long doc's index note and
// its sections, where filenames repeat (every split page has an
// 01-introduction). Doctor flagged all of them as incompatible and suggested
// the bare filename, which is the ambiguous form; it now accepts a link that
// names the note's path. An id-form link is still flagged.
func TestDoctor_APathLinkIsObsidianCompatible(t *testing.T) {
	dir := t.TempDir()
	db, err := index.Open(dir + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, n := range [][2]string{
		{"guide", "imported/web/example.com/guide.md"},
		{"guide-01-introduction", "imported/web/example.com/guide/01-introduction.md"},
		{"other", "concepts/other.md"},
	} {
		_, err = db.Exec("INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
			n[0], n[1], n[0], "h", 0, true)
		require.NoError(t, err)
	}
	for _, l := range [][3]string{
		{"guide", "guide-01-introduction", "imported/web/example.com/guide/01-introduction"},
		{"guide-01-introduction", "guide", "imported/web/example.com/guide"},
		{"other", "guide-01-introduction", "guide-01-introduction"},
	} {
		_, err = db.Exec(`INSERT INTO links (src_note_id, dst_note_id, dst_raw, edge_type, resolved, confidence)
			VALUES (?, ?, ?, 'explicit_link', TRUE, 'high')`, l[0], l[1], l[2])
		require.NoError(t, err)
	}

	result, err := query.Doctor(db, dir, schema.NewRegistry(map[string]vault.TypeDef{}))
	require.NoError(t, err)
	require.Len(t, result.Issues.IncompatibleLinkDetails, 1, "only the id-form link is incompatible")
	assert.Equal(t, "guide-01-introduction", result.Issues.IncompatibleLinkDetails[0].TargetRaw)
}
