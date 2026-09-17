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

// citationVault builds the exact shape from issue #137's reproduction: a
// journal note, and a curated note citing it through source_ids.
func citationVault(t *testing.T, journalAuthoritative string) (*index.DB, *vault.Config) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [source_ids]
  journal:
    required: [title]
`+journalAuthoritative+`
`), 0o600))

	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("raw.md", "---\nid: journal-raw\ntype: journal\ntitle: A scratch entry\n---\nhalf-finished thought\n")
	write("cites.md", "---\nid: concept-cites-raw\ntype: concept\ntitle: Cites raw\nsource_ids: journal-raw\n---\nbody\n")

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, ".vaultmind", "index.db")
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, cfg
}

// THE DEFECT: a curated note citing raw material as evidence.
func TestFindNonAuthoritativeCitations_FlagsRawMaterialCitedAsEvidence(t *testing.T) {
	db, cfg := citationVault(t, "    authoritative: false")

	got, err := query.FindNonAuthoritativeCitations(db, cfg)
	require.NoError(t, err)

	require.Len(t, got, 1, "the citation of a non-authoritative type must be found")
	assert.Equal(t, "concept-cites-raw", got[0].NoteID)
	assert.Equal(t, "journal-raw", got[0].SourceID,
		"the warning must name BOTH notes — a count alone is not actionable")
	assert.Equal(t, "journal", got[0].SourceType)
}

// A vault that has not declared any type non-authoritative sees nothing. The
// field restricts; it must never tighten a vault that did not ask.
func TestFindNonAuthoritativeCitations_SilentWhenNothingIsRestricted(t *testing.T) {
	db, cfg := citationVault(t, "")

	got, err := query.FindNonAuthoritativeCitations(db, cfg)
	require.NoError(t, err)
	assert.Empty(t, got, "no declaration means no opinion — existing vaults must be unaffected")
}

// related_ids is an association, not a claim of evidence. It shares the same
// edge type as source_ids, so filtering on origin is what keeps this check
// about citation rather than about linking.
func TestFindNonAuthoritativeCitations_IgnoresNonCitationRelations(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [related_ids]
  journal:
    required: [title]
    authoritative: false
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "raw.md"),
		[]byte("---\nid: journal-raw\ntype: journal\ntitle: Scratch\n---\nx\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rel.md"),
		[]byte("---\nid: concept-rel\ntype: concept\ntitle: Relates\nrelated_ids: journal-raw\n---\nx\n"), 0o600))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, ".vaultmind", "index.db")
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	got, err := query.FindNonAuthoritativeCitations(db, cfg)
	require.NoError(t, err)
	assert.Empty(t, got,
		"relating to raw material is fine; only CITING it as evidence is the defect")
}
