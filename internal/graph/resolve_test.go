package graph_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVaultPath = "../../vaultmind-vault"

func buildTestDB(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	cfg, err := vault.LoadConfig(testVaultPath)
	require.NoError(t, err)

	idxr := index.NewIndexer(testVaultPath, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err)

	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestResolve_ByExactID(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	result, err := r.Resolve("concept-act-r")
	require.NoError(t, err)

	assert.True(t, result.Resolved)
	assert.False(t, result.Ambiguous)
	assert.Equal(t, "id", *result.ResolutionTier)
	require.Len(t, result.Matches, 1)
	assert.Equal(t, "concept-act-r", result.Matches[0].ID)
	assert.Equal(t, "concept", result.Matches[0].Type)
	assert.Equal(t, "ACT-R", result.Matches[0].Title)
}

func TestResolve_ByExactTitle(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	result, err := r.Resolve("ACT-R")
	require.NoError(t, err)

	assert.True(t, result.Resolved)
	assert.Equal(t, "title", *result.ResolutionTier)
	assert.Equal(t, "concept-act-r", result.Matches[0].ID)
}

func TestResolve_ByAlias(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	result, err := r.Resolve("ACT-R Architecture")
	require.NoError(t, err)

	assert.True(t, result.Resolved)
	assert.Equal(t, "alias", *result.ResolutionTier)
	assert.Equal(t, "concept-act-r", result.Matches[0].ID)
}

func TestResolve_ByNormalized(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	// "act-r architecture" — lowercase of an alias
	result, err := r.Resolve("act-r architecture")
	require.NoError(t, err)

	assert.True(t, result.Resolved)
	assert.Equal(t, "normalized", *result.ResolutionTier)
}

func TestResolve_Unresolved(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	result, err := r.Resolve("NonExistentNoteThatDoesNotExist")
	require.NoError(t, err)

	assert.False(t, result.Resolved)
	assert.Nil(t, result.ResolutionTier)
	assert.Empty(t, result.Matches)
}

func TestResolve_Ambiguous(t *testing.T) {
	// Build a temp DB with two notes sharing a title
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Insert two notes with the same title
	rec1 := index.NoteRecord{
		ID: "proj-alpha", Path: "projects/alpha.md", Title: "Alpha Project",
		Type: "project", Hash: "aaa", MTime: 1, IsDomain: true,
	}
	rec2 := index.NoteRecord{
		ID: "proj-alpha-2", Path: "projects/alpha2.md", Title: "Alpha Project",
		Type: "project", Hash: "bbb", MTime: 1, IsDomain: true,
	}
	require.NoError(t, index.StoreNote(db, rec1))
	require.NoError(t, index.StoreNote(db, rec2))

	r := graph.NewResolver(db)
	result, err := r.Resolve("Alpha Project")
	require.NoError(t, err)

	assert.True(t, result.Resolved)
	assert.True(t, result.Ambiguous)
	assert.Equal(t, "title", *result.ResolutionTier)
	assert.Len(t, result.Matches, 2)
}

func TestResolve_PathShortcut(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	result, err := r.Resolve("concepts/act-r.md")
	require.NoError(t, err)

	assert.True(t, result.Resolved)
	assert.Equal(t, "concept-act-r", result.Matches[0].ID)
}

func TestResolve_Input(t *testing.T) {
	db := buildTestDB(t)
	r := graph.NewResolver(db)

	result, err := r.Resolve("concept-act-r")
	require.NoError(t, err)

	assert.Equal(t, "concept-act-r", result.Input)
}
