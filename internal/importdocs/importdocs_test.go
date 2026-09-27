package importdocs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// srcRepo makes a repository folder holding two docs and an image.
func srcRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "demo-repo")
	write(t, filepath.Join(repo, "docs", "alpha.md"), "# Alpha Guide\n\nHow alpha works.\n")
	write(t, filepath.Join(repo, "docs", "sub", "beta.md"), "No heading here.\n")
	write(t, filepath.Join(repo, "docs", "image.png"), "not markdown")
	return repo
}

func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func source(repo string) importdocs.Source {
	return importdocs.Source{Dir: filepath.Join(repo, "docs"), Repo: "demo-repo", Prefix: "docs"}
}

func run(t *testing.T, repo, vault string, opts importdocs.Options) *importdocs.Result {
	t.Helper()
	res, err := importdocs.Import(source(repo), vault, opts)
	require.NoError(t, err)
	return res
}

func TestImport_EachDocBecomesANoteTiedToItsSource(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Added))

	raw, err := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")) //nolint:gosec // test path
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Equal(t, "imported-demo-repo-docs-alpha", fm["id"])
	assert.Equal(t, "reference", fm["type"])
	assert.Equal(t, "Alpha Guide", fm["title"], "the doc's first heading")
	assert.Equal(t, []interface{}{"demo-repo:docs/alpha.md"}, fm["paths"], "reading the doc brings the note")
	assert.Equal(t, "demo-repo:docs/alpha.md", fm["source"])
	assert.NotEmpty(t, fm["source_hash"])
	assert.Contains(t, body, "How alpha works.")

	raw, err = os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "sub", "beta.md")) //nolint:gosec // test path
	require.NoError(t, err)
	fm, _, err = parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Equal(t, "beta", fm["title"], "no heading: the file name")
}

func TestImport_ARerunChangesOnlyWhatChanged(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	run(t, repo, vault, importdocs.Options{})

	again := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, again.Count(importdocs.Unchanged))
	assert.Zero(t, again.Count(importdocs.Updated))

	write(t, filepath.Join(repo, "docs", "alpha.md"), "# Alpha Guide\n\nHow alpha works now.\n")
	edited := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, edited.Count(importdocs.Updated))
	assert.Equal(t, 1, edited.Count(importdocs.Unchanged))
	raw, _ := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")) //nolint:gosec // test path
	assert.Contains(t, string(raw), "How alpha works now.")
}

// A doc that is gone leaves its note behind, reported; only --prune deletes.
func TestImport_AVanishedDocIsReportedAndPrunedOnlyWhenAsked(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	run(t, repo, vault, importdocs.Options{})
	require.NoError(t, os.Remove(filepath.Join(repo, "docs", "sub", "beta.md")))
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "sub", "beta.md")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Orphaned))
	assert.FileExists(t, note, "nothing is deleted without --prune")

	res = run(t, repo, vault, importdocs.Options{Prune: true})
	assert.Equal(t, 1, res.Count(importdocs.Pruned))
	assert.NoFileExists(t, note)
}

// A note edited by hand is not overwritten when its doc changes: that edit
// would be lost without a word. --force overwrites.
func TestImport_AHandEditIsAConflictNotAnOverwrite(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	run(t, repo, vault, importdocs.Options{})
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")
	raw, _ := os.ReadFile(note) //nolint:gosec // test path
	write(t, note, strings.Replace(string(raw), "How alpha works.", "How alpha works. A hand-written aside.", 1))
	write(t, filepath.Join(repo, "docs", "alpha.md"), "# Alpha Guide\n\nRewritten.\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Conflict))
	kept, _ := os.ReadFile(note) //nolint:gosec // test path
	assert.Contains(t, string(kept), "hand-written aside", "the edit survives")

	res = run(t, repo, vault, importdocs.Options{Force: true})
	assert.Equal(t, 1, res.Count(importdocs.Updated))
	forced, _ := os.ReadFile(note) //nolint:gosec // test path
	assert.Contains(t, string(forced), "Rewritten.")
}

func TestImport_DryRunWritesNothing(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	res := run(t, repo, vault, importdocs.Options{DryRun: true})
	assert.Equal(t, 2, res.Count(importdocs.Added))
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

func TestImport_RefusesADirectoryWithoutMarkdown(t *testing.T) {
	_, err := importdocs.Import(importdocs.Source{Dir: t.TempDir(), Repo: "empty"}, t.TempDir(), importdocs.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no markdown")
}

// Frontmatter a person adds to an imported note (tags, links) is theirs: a
// re-sync refreshes what the import owns and keeps the rest.
func TestImport_KeepsFrontmatterAPersonAdded(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	run(t, repo, vault, importdocs.Options{})
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")
	raw, _ := os.ReadFile(note) //nolint:gosec // test path
	write(t, note, strings.Replace(string(raw), "type: reference\n", "type: reference\ntags: [onboarding]\n", 1))
	write(t, filepath.Join(repo, "docs", "alpha.md"), "# Alpha Guide\n\nRewritten.\n")

	res := run(t, repo, vault, importdocs.Options{})
	require.Equal(t, 1, res.Count(importdocs.Updated), "a frontmatter-only edit is not a conflict")
	raw, _ = os.ReadFile(note) //nolint:gosec // test path
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"onboarding"}, fm["tags"])
	assert.Contains(t, body, "Rewritten.")
}

// A doc's own frontmatter is not note body; its title is the note's title.
func TestImport_ADocsOwnFrontmatterGivesTheTitle(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "gamma.md"), "---\ntitle: Gamma Page\nlayout: page\n---\n# Heading\n\nText.\n")
	run(t, repo, vault, importdocs.Options{})

	raw, _ := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "gamma.md")) //nolint:gosec // test path
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Equal(t, "Gamma Page", fm["title"])
	assert.NotContains(t, body, "layout: page")
	assert.Nil(t, fm["layout"])
}

// A file the import did not write is never overwritten, --force or not.
func TestImport_NeverOverwritesANoteItDidNotWrite(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	mine := filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")
	write(t, mine, "---\nid: my-own\ntype: concept\n---\nMine.\n")

	res := run(t, repo, vault, importdocs.Options{Force: true})
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
	kept, _ := os.ReadFile(mine) //nolint:gosec // test path
	assert.Contains(t, string(kept), "Mine.")
}

// A link is skipped whatever it points at, on both sides: a doc that links
// out of the repository is not read, and a note path that is a link is not
// written through.
func TestImport_SkipsSymlinksAndReportsThem(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.md")
	write(t, secret, "# Secret\n")
	require.NoError(t, os.Symlink(secret, filepath.Join(repo, "docs", "linked.md")))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
	assert.NoFileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "linked.md"))

	target := filepath.Join(t.TempDir(), "outside.md")
	write(t, target, "untouched")
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")
	require.NoError(t, os.Remove(note))
	require.NoError(t, os.Symlink(target, note))
	write(t, filepath.Join(repo, "docs", "alpha.md"), "# Alpha Guide\n\nChanged.\n")

	run(t, repo, vault, importdocs.Options{Force: true})
	out, _ := os.ReadFile(target) //nolint:gosec // test path
	assert.Equal(t, "untouched", string(out))
}

// Importing a folder that holds the vault must not import the vault itself.
func TestImport_SkipsTheVaultWhenItLivesInsideTheSource(t *testing.T) {
	repo := srcRepo(t)
	vault := filepath.Join(repo, "docs", "vault")
	write(t, filepath.Join(vault, "existing.md"), "---\nid: x\ntype: concept\n---\nX.\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Added))
	res = run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Unchanged), "a re-run does not import its own notes")
}

func TestImport_RefusesARepoNameThatIsNotOneSegment(t *testing.T) {
	_, err := importdocs.Import(importdocs.Source{Dir: filepath.Join(srcRepo(t), "docs"), Repo: "../up"}, t.TempDir(), importdocs.Options{})
	require.Error(t, err)
}

// Review 2026-09-27, finding 1: a relative vault path made every existing
// note unrecognised, so hand edits and unmanaged notes were overwritten.
func TestImport_ARelativeVaultPathStillRecognisesExistingNotes(t *testing.T) {
	repo, parent := srcRepo(t), t.TempDir()
	t.Chdir(parent)
	require.NoError(t, os.Mkdir("vault", 0o750))
	opts := importdocs.Options{}
	_, err := importdocs.Import(source(repo), "vault", opts)
	require.NoError(t, err)
	note := filepath.Join(parent, "vault", "imported", "demo-repo", "docs", "alpha.md")
	raw, _ := os.ReadFile(note) //nolint:gosec // test path
	write(t, note, strings.Replace(string(raw), "How alpha works.", "Hand edit.", 1))
	write(t, filepath.Join(repo, "docs", "alpha.md"), "# Alpha Guide\n\nRewritten.\n")

	res, err := importdocs.Import(source(repo), "vault", opts)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Count(importdocs.Conflict))
	assert.Equal(t, 1, res.Count(importdocs.Unchanged))
	assert.Zero(t, res.Count(importdocs.Orphaned))
}

// Finding 2: a note is an orphan only when its own doc is gone from disk —
// not when another import of the same repository put it there, and not when
// this scan merely left its folder out.
func TestImport_OnlyANoteWhoseDocIsGoneIsAnOrphan(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, ".github", "pr.md"), "# PR template\n")
	_, err := importdocs.Import(importdocs.Source{Dir: filepath.Join(repo, ".github"), Repo: "demo-repo", Prefix: ".github"}, vault, importdocs.Options{})
	require.NoError(t, err)

	res, err := importdocs.Import(importdocs.Source{Dir: repo, Repo: "demo-repo"}, vault, importdocs.Options{Prune: true})
	require.NoError(t, err)
	assert.Zero(t, res.Count(importdocs.Pruned), "the root scan skips .github; its doc still exists")
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", ".github", "pr.md"))
}

// Finding 3: a note keeps the id it was given; a new doc whose name slugs to
// the same id gets a distinct one, so no two notes share an id.
func TestImport_ANewDocNeverTakesAnExistingNotesID(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "a-b.md"), "# AB\n")
	run(t, repo, vault, importdocs.Options{})
	write(t, filepath.Join(repo, "docs", "a+b.md"), "# A plus B\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Added))
	assert.Zero(t, res.Count(importdocs.Skipped))
	idOf := func(name string) interface{} {
		raw, _ := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", name)) //nolint:gosec // test path
		fm, _, _ := parser.ExtractFrontmatter(raw)
		return fm["id"]
	}
	assert.Equal(t, "imported-demo-repo-docs-a-b", idOf("a-b.md"), "the existing note keeps its id")
	assert.NotEqual(t, idOf("a-b.md"), idOf("a+b.md"))
	again := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 4, again.Count(importdocs.Unchanged), "ids are stable across runs")
}

// Finding 4: non-ASCII names keep their letters, so they do not collapse.
func TestImport_NonASCIINamesKeepDistinctIDs(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "中文", "说明.md"), "# 说明\n")
	write(t, filepath.Join(repo, "docs", "中文", "指南.md"), "# 指南\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 4, res.Count(importdocs.Added))
	assert.Zero(t, res.Count(importdocs.Skipped))
}

// Finding 5: a note the index would never read is not reported as added. A
// README is renamed so the index reads it; a target the vault excludes is
// skipped with the reason; the note always gets a lowercase .md.
func TestImport_WritesOnlyNotesTheIndexReads(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "README.md"), "# Docs overview\n")
	write(t, filepath.Join(repo, "docs", "UP.MD"), "# Up\n")
	write(t, filepath.Join(repo, "docs", "episodes", "e.md"), "# Episode\n")

	res, err := importdocs.Import(source(repo), vault, importdocs.Options{Excludes: []string{"README.md", "episodes"}})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "readme.md"))
	assert.FileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "UP.md"))
	assert.NoFileExists(t, filepath.Join(vault, "imported", "demo-repo", "docs", "episodes", "e.md"))
	assert.Equal(t, 4, res.Count(importdocs.Added))
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
}

// Finding 6: the vault is never a source, and a source that holds the vault
// skips it however the path is spelled.
func TestImport_RefusesToImportTheVaultIntoItself(t *testing.T) {
	vault := t.TempDir()
	write(t, filepath.Join(vault, "n.md"), "# N\n")
	_, err := importdocs.Import(importdocs.Source{Dir: vault, Repo: "v"}, vault, importdocs.Options{})
	require.Error(t, err)
	_, err = importdocs.Import(importdocs.Source{Dir: filepath.Join(vault, "sub"), Repo: "v"}, vault, importdocs.Options{})
	require.Error(t, err, "a folder inside the vault is already in the vault")

	repo := srcRepo(t)
	link := filepath.Join(t.TempDir(), "link-to-docs")
	require.NoError(t, os.Symlink(filepath.Join(repo, "docs"), link))
	inside := filepath.Join(repo, "docs", "kv")
	write(t, filepath.Join(inside, "x.md"), "# X\n")
	res, err := importdocs.Import(importdocs.Source{Dir: link, Repo: "demo-repo", Prefix: "docs"}, inside, importdocs.Options{})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Count(importdocs.Added), "the vault is skipped when reached through a link")
}

// Finding 7: a doc path with a control character is skipped — it would
// otherwise write its own frontmatter lines into the note.
func TestImport_SkipsADocPathWithControlCharacters(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "x\naliases: [Injected]\n#.md"), "# X\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Added))
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
}

// Finding 8: dependency folders are not docs.
func TestImport_SkipsDependencyFolders(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "node_modules", "pkg", "README.md"), "# pkg\n")
	write(t, filepath.Join(repo, "docs", "vendor", "lib", "CHANGELOG.md"), "# lib\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Added))
}

// A hand-edited orphan is not pruned without --force: the edit is the only
// copy left.
func TestImport_AHandEditedOrphanIsNotPrunedWithoutForce(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	run(t, repo, vault, importdocs.Options{})
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "sub", "beta.md")
	raw, _ := os.ReadFile(note) //nolint:gosec // test path
	write(t, note, string(raw)+"My notes.\n")
	require.NoError(t, os.Remove(filepath.Join(repo, "docs", "sub", "beta.md")))

	res := run(t, repo, vault, importdocs.Options{Prune: true})
	assert.Equal(t, 1, res.Count(importdocs.Conflict))
	assert.FileExists(t, note)
	res = run(t, repo, vault, importdocs.Options{Prune: true, Force: true})
	assert.Equal(t, 1, res.Count(importdocs.Pruned))
}

// A link in the vault above the imported notes is refused before anything
// is read through it.
func TestImport_RefusesALinkAboveTheImportedNotes(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	elsewhere := t.TempDir()
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(vault, "imported")))

	_, err := importdocs.Import(source(repo), vault, importdocs.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

// A note whose frontmatter does not parse is not the import's: it is left
// alone, whatever --force says.
func TestImport_LeavesANoteWithBrokenFrontmatterAlone(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	broken := filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")
	write(t, broken, "---\nid: [unclosed\n---\nMine.\n")

	res := run(t, repo, vault, importdocs.Options{Force: true})
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
	kept, _ := os.ReadFile(broken) //nolint:gosec // test path
	assert.Contains(t, string(kept), "Mine.")
}

// Re-review finding 1: names that differ only in case are one file on the
// default macOS filesystem. A hand-written note at the other case is not
// overwritten, and a managed one keeps its conflict check and is renamed to
// the name the doc now gives it.
func TestImport_ANoteDifferingOnlyInCaseIsTheSameNote(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "README.md"), "# Overview\n")
	mine := filepath.Join(vault, "imported", "demo-repo", "docs", "README.md")
	write(t, mine, "---\nid: my-readme\ntype: concept\n---\nMine.\n")

	res := run(t, repo, vault, importdocs.Options{Force: true})
	assert.Equal(t, 1, res.Count(importdocs.Skipped), "the hand-written README.md is in the way of readme.md")
	kept, _ := os.ReadFile(mine) //nolint:gosec // test path
	assert.Contains(t, string(kept), "Mine.")
}

func TestImport_ARenamedDocKeepsItsConflictCheck(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "Foo.md"), "# Foo\n\nOne.\n")
	run(t, repo, vault, importdocs.Options{})
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "Foo.md")
	raw, _ := os.ReadFile(note) //nolint:gosec // test path
	write(t, note, string(raw)+"HAND\n")
	require.NoError(t, os.Remove(filepath.Join(repo, "docs", "Foo.md")))
	write(t, filepath.Join(repo, "docs", "foo.md"), "# Foo\n\nTwo.\n")

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Conflict))
	kept, _ := os.ReadFile(note) //nolint:gosec // test path
	assert.Contains(t, string(kept), "HAND")

	res = run(t, repo, vault, importdocs.Options{Force: true})
	assert.Equal(t, 1, res.Count(importdocs.Updated))
	names := listDir(t, filepath.Join(vault, "imported", "demo-repo", "docs"))
	assert.Contains(t, names, "foo.md", "renamed to the doc's new name")
	assert.NotContains(t, names, "Foo.md")
}

// Two docs whose names differ only in case cannot both be notes: on a
// case-insensitive filesystem they are one file. The second is skipped on
// every platform, so a vault behaves the same wherever it lives.
func TestImport_DocsDifferingOnlyInCaseAreOneNote(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "Up.md"), "# Up\n")
	if _, err := os.Stat(filepath.Join(repo, "docs", "up.md")); err == nil {
		t.Skip("case-insensitive filesystem: the two docs cannot coexist")
	}
	write(t, filepath.Join(repo, "docs", "up.md"), "# up\n")
	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Skipped))
}

// Re-review finding 2: ids are unique across every import in the vault, not
// only this folder's.
func TestImport_IDsAreUniqueAcrossImportsOfOneRepo(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "a-b.md"), "# AB\n")
	write(t, filepath.Join(repo, "docs-a", "b.md"), "# B\n")
	run(t, repo, vault, importdocs.Options{})
	_, err := importdocs.Import(importdocs.Source{Dir: filepath.Join(repo, "docs-a"), Repo: "demo-repo", Prefix: "docs-a"}, vault, importdocs.Options{})
	require.NoError(t, err)

	idOf := func(rel string) interface{} {
		raw, _ := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", rel)) //nolint:gosec // test path
		fm, _, _ := parser.ExtractFrontmatter(raw)
		return fm["id"]
	}
	assert.NotEqual(t, idOf("docs/a-b.md"), idOf("docs-a/b.md"))
}

// Re-review finding 3: a doc renamed without a content change refreshes the
// note's source and paths, which would otherwise name a file that is gone.
func TestImport_ARenameWithoutAChangeRefreshesTheSource(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "UP.MD"), "# Up\n")
	run(t, repo, vault, importdocs.Options{})
	require.NoError(t, os.Rename(filepath.Join(repo, "docs", "UP.MD"), filepath.Join(repo, "docs", "tmp")))
	require.NoError(t, os.Rename(filepath.Join(repo, "docs", "tmp"), filepath.Join(repo, "docs", "UP.md")))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Updated))
	raw, _ := os.ReadFile(filepath.Join(vault, "imported", "demo-repo", "docs", "UP.md")) //nolint:gosec // test path
	assert.Contains(t, string(raw), "source: demo-repo:docs/UP.md")
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// Review round 3, low 1: a note another import wrote that cannot be read is
// not this import's problem. It only blocks the import whose folder it is in.
func TestImport_AnUnreadableNoteElsewhereDoesNotBlock(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads a 0000 file")
	}
	repo, vault := srcRepo(t), t.TempDir()
	other := filepath.Join(vault, "imported", "other-repo", "docs", "x.md")
	write(t, other, "---\nid: x\n---\nX.\n")
	require.NoError(t, os.Chmod(other, 0))
	t.Cleanup(func() { _ = os.Chmod(other, 0o600) })

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 2, res.Count(importdocs.Added))

	mine := filepath.Join(vault, "imported", "demo-repo", "docs", "alpha.md")
	require.NoError(t, os.Chmod(mine, 0))
	t.Cleanup(func() { _ = os.Chmod(mine, 0o600) })
	_, err := importdocs.Import(source(repo), vault, importdocs.Options{})
	require.Error(t, err, "an unreadable note in this import's own folder still stops it")
}

// Review round 3, low 2: a doc renamed with its content unchanged refreshes
// the note's source and keeps a hand-edited body — the doc did not change,
// so there is nothing to choose between.
func TestImport_ARenamedUnchangedDocKeepsTheHandEditedBody(t *testing.T) {
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "UP.MD"), "# Up\n\nText.\n")
	run(t, repo, vault, importdocs.Options{})
	note := filepath.Join(vault, "imported", "demo-repo", "docs", "UP.md")
	raw, _ := os.ReadFile(note) //nolint:gosec // test path
	write(t, note, string(raw)+"HAND\n")
	require.NoError(t, os.Rename(filepath.Join(repo, "docs", "UP.MD"), filepath.Join(repo, "docs", "tmp")))
	require.NoError(t, os.Rename(filepath.Join(repo, "docs", "tmp"), filepath.Join(repo, "docs", "UP.md")))

	res := run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Updated))
	assert.Zero(t, res.Count(importdocs.Conflict))
	raw, _ = os.ReadFile(note) //nolint:gosec // test path
	assert.Contains(t, string(raw), "source: demo-repo:docs/UP.md")
	assert.Contains(t, string(raw), "HAND", "the hand edit survives")

	write(t, filepath.Join(repo, "docs", "UP.md"), "# Up\n\nChanged.\n")
	res = run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, res.Count(importdocs.Conflict), "still counted as edited: a later doc change is a conflict")
}
