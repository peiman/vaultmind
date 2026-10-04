package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docsRepo is a repository with no remote (so it is named by its folder)
// holding a docs folder of two docs.
func docsRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "demo-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "docs"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "alpha.md"), []byte("# Alpha Guide\n\nHow alpha works.\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "beta.md"), []byte("# Beta\n\nBeta text.\n"), 0o600))
	return repo
}

// An imported doc is a note the vault can serve, and the code hooks find it
// from the doc: tree --for the doc lists its note.
func TestImport_DocsBecomeIndexedNotesFoundFromTheDoc(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Imported demo-repo:docs into")
	assert.Contains(t, out.String(), "2 added")

	out, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-docs-alpha", "--vault", vault, "--json")
	require.NoError(t, err, "the import indexed what it wrote")
	assert.Contains(t, out.String(), "How alpha works.")

	out, _, err = runRootCmd(t, "tree", "--for", filepath.Join(repo, "docs", "alpha.md"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "imported-demo-repo-docs-alpha")
}

func TestImport_JSONReportsEachDocAndARerunIsUnchanged(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	_, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Counts  map[string]int `json:"counts"`
			Entries []struct {
				Action string `json:"action"`
				Note   string `json:"note"`
				Source string `json:"source"`
			} `json:"entries"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, map[string]int{"unchanged": 2}, env.Result.Counts)
	require.Len(t, env.Result.Entries, 2)
	assert.Equal(t, "imported/demo-repo/docs/alpha.md", env.Result.Entries[0].Note)
	assert.Equal(t, "demo-repo:docs/alpha.md", env.Result.Entries[0].Source)
}

func TestImport_DryRunWritesNothing(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "dry run, nothing written")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

// A vanished doc's note is listed with what to do about it, and --prune
// removes it from the vault and the index.
func TestImport_OrphansAreListedAndPrunedOnRequest(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	_, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(repo, "docs", "beta.md")))

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "orphaned  imported/demo-repo/docs/beta.md — its doc is gone; --prune removes it")

	_, _, err = runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault, "--prune")
	require.NoError(t, err)
	_, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-docs-beta", "--vault", vault, "--json")
	assert.Error(t, err, "pruned from the index too")
}

// The repository root itself can be imported: its docs are named from the
// root, with no prefix.
func TestImport_TheRepositoryRootNamesDocsFromTheRoot(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Demo\n"), 0o600))

	out, _, err := runRootCmd(t, "import", repo, "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), `"source":"demo-repo",`, "the root is named by the repository alone")
	assert.Contains(t, out.String(), `"source":"demo-repo:README.md"`)
	assert.Contains(t, out.String(), `"source":"demo-repo:docs/alpha.md"`)
}

func TestImport_AFolderWithoutMarkdownIsAJSONError(t *testing.T) {
	vault := indexedBaselineVault(t)

	out, _, err := runRootCmd(t, "import", t.TempDir(), "--vault", vault, "--json")
	require.Error(t, err)
	assert.Contains(t, out.String(), `"no_markdown"`)
}

// A file is imported only when it is markdown, a PDF or an Office document;
// anything else is refused with what is accepted (before PDFs and single
// files, every file was refused).
func TestImport_RefusesAFileThatIsNotMarkdownOrPDF(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "notes.txt"), []byte("text"), 0o600))

	_, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs", "notes.txt"), "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".md, .pdf, .docx, .pptx, .xlsx, .html, .htm, .csv, .tsv, .epub, .zip, .tar, .tar.gz, .tgz or image (.png, .jpg, .jpeg, .webp, .gif, .heic, .heif, .svg)")
}

// On a re-sync of a big folder the one doc that changed is named, not lost in
// a count.
func TestImport_NamesTheChangedNoteAmongManyUnchanged(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	for i := range importListLimit + 5 {
		name := filepath.Join(repo, "docs", fmt.Sprintf("extra-%02d.md", i))
		require.NoError(t, os.WriteFile(name, []byte("# Extra\n"), 0o600))
	}
	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "added     imported/", "a big first import is a count, not a list")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "alpha.md"), []byte("# Alpha Guide\n\nChanged.\n"), 0o600))
	out, _, err = runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "updated   imported/demo-repo/docs/alpha.md")
}

// An import embeds the notes it wrote even when they are more than a single
// write's limit: they were asked for. Only a backlog beyond them is left.
func TestImport_EmbedsWhatItImportedPastTheWriteLimit(t *testing.T) {
	vault, repo := miniLMVault(t), docsRepo(t)
	stubEmbedPass(t, &index.EmbedResult{Embedded: 2}, nil)
	saved := embedOnWriteLimit
	embedOnWriteLimit = 1
	t.Cleanup(func() { embedOnWriteLimit = saved })

	_, errOut, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "embedded 2 note(s)")
	assert.NotContains(t, errOut.String(), "notes have no embeddings")
}

// A note the index did not take is named in the report: written is not the
// same as findable. Here a hand-written note already holds the id the import
// gives its doc.
func TestImport_ReportsANoteTheIndexDidNotTake(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(vault, "squatter.md"),
		[]byte("---\nid: imported-demo-repo-docs-alpha\ntype: concept\n---\nMine.\n"), 0o600))
	_, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "warning   imported/demo-repo/docs/alpha.md not indexed: id imported-demo-repo-docs-alpha is already held by squatter.md")
}

// The README of a docs folder is its overview: it is imported under a name
// the vault's README exclusion does not hide, and the index takes it.
func TestImport_AFoldersReadmeIsIndexed(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "README.md"), []byte("# Docs overview\n\nStart here.\n"), 0o600))

	_, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	out, _, err := runRootCmd(t, "note", "get", "imported-demo-repo-docs-readme", "--vault", vault, "--json")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Start here.")
}

// A relative --vault, as the help examples use, recognises the notes a
// previous import wrote (review finding 1, at the command).
func TestImport_ARelativeVaultPathReRunsUnchanged(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	t.Chdir(filepath.Dir(vault))
	rel := filepath.Base(vault)
	_, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", rel)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", rel)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "2 unchanged")
}

// A scheme other than http or https is rejected before the vault is opened
// and before any network call. file:// must not be read as a document.
func TestImport_RejectsNonHTTPURLsBeforeAnyWork(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-vault")
	for _, arg := range []string{"ftp://x", "file:///etc/passwd", "HTTP://example.com/x"} {
		_, _, err := runRootCmd(t, "import", arg, "--vault", missing)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only http and https URLs can be imported")
		assert.NotContains(t, err.Error(), "no-such-vault")
	}
	assert.NoDirExists(t, missing)
}

// import of one httptest page writes the note and indexes it.
func TestImport_AURLBecomesAnIndexedNote(t *testing.T) {
	vault := indexedBaselineVault(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, "# From The Web\n\nIndexed page text.\n")
	}))
	t.Cleanup(srv.Close)
	pageURL := srv.URL + "/guide.md"

	out, _, err := runRootCmd(t, "import", pageURL, "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Source  string         `json:"source"`
			Counts  map[string]int `json:"counts"`
			Entries []struct {
				Action string `json:"action"`
				Note   string `json:"note"`
				Source string `json:"source"`
			} `json:"entries"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, pageURL, env.Result.Source)
	assert.Equal(t, map[string]int{"added": 1}, env.Result.Counts)
	require.Len(t, env.Result.Entries, 1)
	assert.Equal(t, "added", env.Result.Entries[0].Action)
	assert.Equal(t, pageURL, env.Result.Entries[0].Source)
	assert.Contains(t, env.Result.Entries[0].Note, "imported/web/")

	raw, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(env.Result.Entries[0].Note))) //nolint:gosec // vault path from the test
	require.NoError(t, err)
	fm, body, err := parser.ExtractFrontmatter(raw)
	require.NoError(t, err)
	assert.Contains(t, body, "Indexed page text.")
	id, _ := fm["id"].(string)
	require.NotEmpty(t, id)

	out, _, err = runRootCmd(t, "note", "get", id, "--vault", vault, "--json")
	require.NoError(t, err, "the import indexed what it wrote")
	assert.Contains(t, out.String(), "Indexed page text.")
}

func TestImport_URLDryRunFetchesButWritesNothing(t *testing.T) {
	vault := indexedBaselineVault(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "Hello.\n")
	}))
	t.Cleanup(srv.Close)

	out, _, err := runRootCmd(t, "import", srv.URL+"/hello", "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.Equal(t, 1, hits, "a dry run still fetches")
	assert.Contains(t, out.String(), "dry run, nothing written")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

// One file imports alone: a markdown doc or a PDF, named by its path.
func TestImport_OneFileImportsAlone(t *testing.T) {
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	pdf, err := os.ReadFile(filepath.Join("..", "internal", "importdocs", "testdata", "paper.pdf"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "paper.pdf"), pdf, 0o600))

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs", "paper.pdf"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "1 added")
	assert.Contains(t, out.String(), "paper-pdf.md")

	out, _, err = runRootCmd(t, "import", filepath.Join(repo, "docs", "alpha.md"), "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "1 added")
	assert.NotContains(t, out.String(), "orphan", "a single-file import never reports the folder's other notes")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "style.css"), []byte("body {}"), 0o600))
	_, _, err = runRootCmd(t, "import", filepath.Join(repo, "docs", "style.css"), "--vault", vault)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".md, .pdf, .docx, .pptx, .xlsx, .html, .htm, .csv, .tsv, .epub, .zip, .tar, .tar.gz, .tgz or image (.png, .jpg, .jpeg, .webp, .gif, .heic, .heif, .svg)")
}

// --public-only is for callers that must not reach this machine or its
// network: an MCP client, which a page it read can prompt. It imports only
// http(s) URLs, and only from public addresses, even a URL naming
// localhost, which a plain import allows because the operator chose it.
func TestImport_PublicOnlyRefusesLocalPathsAndPrivateHosts(t *testing.T) {
	vault := indexedBaselineVault(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "Internal page.\n")
	}))
	t.Cleanup(srv.Close)

	_, _, err := runRootCmd(t, "import", srv.URL+"/admin", "--vault", vault, "--public-only")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to connect to 127.0.0.1")
	assert.Zero(t, hits, "the private address is never reached")

	docs := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(docs, "secret.md"), []byte("# Secret\n\nKeep out.\n"), 0o600))
	_, _, err = runRootCmd(t, "import", docs, "--vault", vault, "--public-only")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only http(s) URLs")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}
