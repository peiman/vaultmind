package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildBigBodyVault creates a minimal vault with one note whose body is
// big enough that a mid-sized budget cannot fit it. The vault has just one
// note so ContextPack doesn't spend any budget on context items.
func buildBigBodyVault(t *testing.T) *index.DB {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	// ~8000 chars = ~2000 tokens
	bigBody := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 200)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.md"), []byte(`---
id: concept-big
type: concept
title: Big Note
---
`+bigBody), 0o644))

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ContextPack's packTargetContent has a body-truncation path that fires
// when frontmatter tokens ≤ budget but body tokens > remaining budget.
// Existing tests hit the "frontmatter-exceeds-budget" path (Budget: 10);
// this test covers the truncated-body path that keeps a prefix of the body
// in the pack. Agents rely on this behavior to get useful context even
// under budget pressure.
func TestContextPack_BodyTruncationPrefixIncluded(t *testing.T) {
	db := buildBigBodyVault(t)
	resolver := graph.NewResolver(db)

	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "concept-big", Budget: 100, MaxItems: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Target)

	assert.True(t, result.Truncated, "body >> remaining budget must set Truncated=true")
	assert.NotEmpty(t, result.Target.Body, "truncated prefix must still be included")
	// The body was ~8000 chars; the truncated version must be substantially
	// shorter (remaining * 4 bytes per token, with budget 100 - fm tokens ≈ 380-400 chars)
	assert.Less(t, len(result.Target.Body), 2000,
		"truncated body must be far smaller than the 8000-char original")
}

// Zero body (empty note body) takes the else-branch in packTargetContent:
// Truncated is set but the target body remains empty. This is the edge
// case where a note has frontmatter but no body content.
func TestContextPack_EmptyBodyWithLargeFrontmatter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	// Body intentionally empty after the closing --- (just a newline)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.md"), []byte(`---
id: concept-empty
type: concept
title: Empty Body
tags: [one, two, three, four, five, six, seven, eight, nine, ten]
---
`), 0o644))
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	resolver := graph.NewResolver(db)
	// Budget just enough for frontmatter, nothing left for body (body is
	// empty anyway). Regression: the empty-body branch in packTargetContent
	// must not panic and must return a non-nil target.
	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "concept-empty", Budget: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Target)
	assert.Empty(t, result.Target.Body, "empty-body note has no body to include")
}
