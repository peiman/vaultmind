package index_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smallTitlesVault builds a minimal vault + index containing three domain
// notes. Returns the open DB; test cleanup closes it.
func smallTitlesVault(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.md"), []byte(`---
id: concept-alpha
type: concept
title: The Judgment Gap
---
body one
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "beta.md"), []byte(`---
id: concept-beta
type: concept
title: Spreading Activation
aliases: [spread-act]
---
body two
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gamma.md"), []byte(`---
id: concept-gamma
type: concept
title: Reality Is The Spec
---
body three
`), 0o644))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// AllNoteTitles must return every indexed note's ID and title — the ask
// command's zero-hit fuzzy-title fallback depends on getting EVERY note
// back, not a filtered subset.
func TestAllNoteTitles_ReturnsEveryIndexedNote(t *testing.T) {
	db := smallTitlesVault(t)
	titles, err := db.AllNoteTitles()
	require.NoError(t, err)
	assert.Len(t, titles, 3)

	titleByID := map[string]string{}
	for _, nt := range titles {
		titleByID[nt.ID] = nt.Title
	}
	assert.Equal(t, "The Judgment Gap", titleByID["concept-alpha"])
	assert.Equal(t, "Spreading Activation", titleByID["concept-beta"])
	assert.Equal(t, "Reality Is The Spec", titleByID["concept-gamma"])
}

// QueryNotesByNormalized matches a title with hyphens/underscores replaced
// with spaces, case-insensitive. "the judgment gap" should match
// "The Judgment Gap". Regression: losing the normalization step would
// break wikilink resolution that uses hyphenated file slugs.
func TestQueryNotesByNormalized_MatchesTitleIgnoringCaseAndHyphens(t *testing.T) {
	db := smallTitlesVault(t)
	rows, err := db.QueryNotesByNormalized("the judgment gap")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "concept-alpha", rows[0].ID)
}

// Alias-normalized matching: "spread act" (with space) should reach the
// beta note via its "spread-act" alias. The aliases table normalizes the
// hyphen away.
func TestQueryNotesByNormalized_MatchesAliasWithSeparatorFolded(t *testing.T) {
	db := smallTitlesVault(t)
	rows, err := db.QueryNotesByNormalized("spread act")
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	var gotBeta bool
	for _, r := range rows {
		if r.ID == "concept-beta" {
			gotBeta = true
		}
	}
	assert.True(t, gotBeta, "alias spread-act must match 'spread act' via normalization")
}

// Non-existent query returns an empty slice and no error — callers rely
// on the zero-result path not being an error.
func TestQueryNotesByNormalized_UnknownReturnsEmpty(t *testing.T) {
	db := smallTitlesVault(t)
	rows, err := db.QueryNotesByNormalized("not in vault")
	require.NoError(t, err)
	assert.Empty(t, rows)
}
