package mutation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildLintTestDB sets up an in-memory SQLite DB with necessary schema and seed data.
// It opens a temp DB, applies migrations via index.Open, then seeds the note records.
func buildLintTestDB(t *testing.T) (*index.DB, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func TestFixWikilinks_RewritesTargetToFilename(t *testing.T) {
	db, dir := buildLintTestDB(t)

	// Create vault directory structure
	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	// Seed target note in DB: title "My Target" lives at concepts/my-target.md
	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-my-target", "concepts/my-target.md", "My Target", "abc123", 0, true,
	)
	require.NoError(t, err)

	// Create source note file with an incompatible wikilink
	noteContent := "---\nid: concept-test\ntype: concept\ntitle: Test\ncreated: 2026-04-07\n---\n\nSee [[My Target]] for details.\n"
	notePath := filepath.Join(vaultDir, "concepts", "test.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.FilesScanned)
	assert.Equal(t, 1, result.FilesChanged)
	assert.Equal(t, 1, result.LinksFixed)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[[my-target|My Target]]")
	assert.NotContains(t, string(content), "[[My Target]]")
}

func TestFixWikilinks_DryRun_DoesNotWriteFile(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-alpha", "concepts/alpha.md", "Alpha Note", "def456", 0, true,
	)
	require.NoError(t, err)

	originalContent := "---\nid: concept-src\ntype: concept\ntitle: Src\n---\n\nSee [[Alpha Note]] here.\n"
	notePath := filepath.Join(vaultDir, "concepts", "src.md")
	require.NoError(t, os.WriteFile(notePath, []byte(originalContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, false) // fix=false → dry run
	require.NoError(t, err)
	assert.Equal(t, 1, result.LinksFixed)

	// File must remain unchanged
	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(content))
}

func TestFixWikilinks_SkipsAlreadyCompatibleLinks(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	// title matches filename stem — already compatible
	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-beta", "concepts/beta.md", "beta", "ghi789", 0, true,
	)
	require.NoError(t, err)

	noteContent := "---\nid: concept-compat\ntype: concept\ntitle: Compat\n---\n\nSee [[beta]] for details.\n"
	notePath := filepath.Join(vaultDir, "concepts", "compat.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.LinksFixed)
	assert.Equal(t, 0, result.FilesChanged)
}

func TestFixWikilinks_SkipsPipeLinks(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-gamma", "concepts/gamma.md", "Gamma Title", "jkl", 0, true,
	)
	require.NoError(t, err)

	// [[gamma|Gamma Title]] already uses pipe format — skip it
	noteContent := "---\nid: concept-pipe\ntype: concept\ntitle: Pipe\n---\n\nSee [[gamma|Gamma Title]] here.\n"
	notePath := filepath.Join(vaultDir, "concepts", "pipe.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.LinksFixed)
}

func TestFixWikilinks_SkipsFrontmatter(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-delta", "concepts/delta.md", "Delta Note", "mno", 0, true,
	)
	require.NoError(t, err)

	// Frontmatter contains the string [[Delta Note]] — must not be rewritten
	noteContent := "---\nid: concept-fm\ntype: concept\ntitle: FM\naliases: [\"[[Delta Note]]\"]\n---\n\nNo link in body.\n"
	notePath := filepath.Join(vaultDir, "concepts", "fm.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)
	assert.Equal(t, 0, result.LinksFixed)

	// Frontmatter must be preserved unchanged
	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[[Delta Note]]")
}

func TestFixWikilinks_RewritesAliasLinks(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	// Insert a note and one of its aliases
	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-zeta", "concepts/zeta.md", "Zeta Title", "pqr", 0, true,
	)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO aliases (note_id, alias, alias_normalized) VALUES (?, ?, ?)",
		"concept-zeta", "Zeta Alias", "zeta alias",
	)
	require.NoError(t, err)

	// Link uses alias, not title — should still be fixed to [[zeta|Zeta Alias]]
	noteContent := "---\nid: concept-alias-src\ntype: concept\ntitle: Src\n---\n\nSee [[Zeta Alias]] here.\n"
	notePath := filepath.Join(vaultDir, "concepts", "alias-src.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)
	assert.Equal(t, 1, result.LinksFixed)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[[zeta|Zeta Alias]]")
	assert.NotContains(t, string(content), "[[Zeta Alias]]")
}

func TestFixWikilinks_MultipleLinksInOneFile(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-one", "concepts/one.md", "One Title", "aaa", 0, true,
	)
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-two", "concepts/two.md", "Two Title", "bbb", 0, true,
	)
	require.NoError(t, err)

	noteContent := "---\nid: concept-multi\ntype: concept\ntitle: Multi\n---\n\nSee [[One Title]] and [[Two Title]] here.\n"
	notePath := filepath.Join(vaultDir, "concepts", "multi.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, true)
	require.NoError(t, err)
	assert.Equal(t, 2, result.LinksFixed)
	assert.Equal(t, 1, result.FilesChanged)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "[[one|One Title]]")
	assert.Contains(t, string(content), "[[two|Two Title]]")
}

func TestFixWikilinks_PopulatesDetails(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, "concepts"), 0o755))

	_, err := db.Exec(
		"INSERT INTO notes (id, path, title, hash, mtime, is_domain) VALUES (?, ?, ?, ?, ?, ?)",
		"concept-eta", "concepts/eta.md", "Eta Note", "xyz", 0, true,
	)
	require.NoError(t, err)

	noteContent := "---\nid: concept-det\ntype: concept\ntitle: Det\n---\n\nSee [[Eta Note]].\n"
	notePath := filepath.Join(vaultDir, "concepts", "det.md")
	require.NoError(t, os.WriteFile(notePath, []byte(noteContent), 0o644))

	result, err := mutation.FixWikilinks(db, vaultDir, false)
	require.NoError(t, err)
	require.Len(t, result.Details, 1)
	assert.Equal(t, "[[Eta Note]]", result.Details[0].OldLink)
	assert.Equal(t, "[[eta|Eta Note]]", result.Details[0].NewLink)
}

func TestFixWikilinks_EmptyVault(t *testing.T) {
	db, dir := buildLintTestDB(t)

	vaultDir := filepath.Join(dir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0o755))

	result, err := mutation.FixWikilinks(db, vaultDir, false)
	require.NoError(t, err)
	assert.Equal(t, 0, result.FilesScanned)
	assert.Equal(t, 0, result.LinksFixed)
}
